package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Debt detection reads the six complete periods before the current one; after
// an import a category needs payments in at least two of them for its median
// to be the debt's payment.
const (
	debtHistoryMonths = 6
	debtMinPaidMonths = 2
	debtNameMax       = 64
)

const debtsSystem = `You help set up a personal budget. Find the expense categories that are repayments of a loan: a mortgage, a car loan, a student loan, a personal loan, the instalments of a credit line. Rent, utilities, subscriptions, insurance and credit-card spending are not loan repayments. Names may be in any language; use the user's timezone and language to read local bank and product names. A category may carry "monthly", what was spent in it in each of the last months, oldest first: a loan is usually repaid in steady amounts. Reply with JSON only, exactly {"debts":[{"categoryId":"<id>","name":"<name>"}]}, listing only the loan repayments, each with a short name for the loan in the user's language (for example "Mortgage"); use an empty list when none is.`

type debtCategory struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Monthly []string `json:"monthly,omitempty"`
}

type debtFacts struct {
	Timezone   string         `json:"timezone"`
	Language   string         `json:"language"`
	Categories []debtCategory `json:"categories"`
}

type debtCandidate struct {
	id       vo.Id
	name     string
	currency vo.Id
	payment  vo.DecimalNumber
}

// startPlanDebts follows save-plan's commit over the planned categories
// (lower-cased id -> amount). A planned category a debt of the plan's budget
// already covers hands its plan to the debt's row at once (the user planned
// the loan, and the category's row hides behind the debt's), with or without
// AI. The active own expense categories planned above zero that no debt of
// the budget covers then go to the debt detection; with none, no job starts
// and no cap slot is used.
func (s *Service) startPlanDebts(ctx context.Context, userID vo.Id, budget string, planned map[string]vo.DecimalNumber) {
	if s.debts == nil || budget == "" || !anyPositive(planned) {
		return
	}
	budgetID := vo.MustParseId(budget)
	covered, err := s.coveredCategories(ctx, budgetID)
	if err != nil {
		slog.WarnContext(ctx, jobDebts.failed, "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		return
	}
	own, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		slog.WarnContext(ctx, jobDebts.failed, "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		return
	}
	detect := map[string]vo.DecimalNumber{}
	for _, c := range own {
		key := strings.ToLower(c.Id)
		if c.IsArchived == 1 || c.Type != model.TypeExpense.Alias() || !positive(planned[key]) {
			continue
		}
		if !covered[key] {
			detect[key] = planned[key]
			continue
		}
		if _, err := s.debts.TakeCategoryPlans(ctx, budgetID, vo.MustParseId(c.Id)); err != nil {
			slog.WarnContext(ctx, "onboarding debt plan handover failed", "user_id", userID.String(), "category_id", c.Id, "err_type", fmt.Sprintf("%T", err))
		}
	}
	if len(detect) == 0 || !s.reserveJob(ctx, userID, jobDebts) {
		return
	}
	s.runJob(userID, jobDebts, s.promptContext(ctx, userID), func(jobCtx context.Context, facts promptFacts) error {
		return s.detectDebts(jobCtx, userID, budgetID, facts, detect)
	})
}

// coveredCategories are the lower-cased ids of the categories the budget's
// debts count in; another budget's debts cover nothing here.
func (s *Service) coveredCategories(ctx context.Context, budgetID vo.Id) (map[string]bool, error) {
	ids, err := s.debts.CoveredCategories(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[strings.ToLower(id.String())] = true
	}
	return out, nil
}

// detectDebts asks the AI which of the user's active own expense categories
// repay a loan and creates a debt, not set up and paid monthly, for each one it
// names. A category a debt of the budget already covers is never asked about.
// The payment is the category's plan when planned is set (a category without a
// plan above zero is not asked about), else the median of its paid months in the history
// (fewer than two: not asked about), in the category's element currency. A
// category whose currency cannot be read, or that the debt feature refuses, is
// skipped. An unusable reply creates nothing.
func (s *Service) detectDebts(ctx context.Context, userID, budgetID vo.Id, facts promptFacts, planned map[string]vo.DecimalNumber) error {
	if loc, err := time.LoadLocation(facts.Timezone); err == nil && facts.Timezone != "" {
		ctx = reqctx.WithLocation(ctx, loc)
	}
	covered, err := s.coveredCategories(ctx, budgetID)
	if err != nil {
		return err
	}
	own, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		return err
	}
	history, err := s.steps.Budgets.CategoryHistory(ctx, userID, budgetID, debtHistoryMonths)
	if err != nil {
		return err
	}
	byID := map[string]model.BudgetCategoryHistory{}
	for _, h := range history {
		if !h.IsIncome {
			byID[strings.ToLower(h.CategoryID)] = h
		}
	}
	budgetCurrency := ""
	currencyOf := func(key string) (string, error) {
		if h, ok := byID[key]; ok {
			return h.CurrencyID, nil
		}
		if budgetCurrency == "" {
			_, cur, _, err := s.steps.Budgets.UnplannedOwnBudget(ctx, userID, budgetID)
			if err != nil {
				return "", err
			}
			budgetCurrency = cur
		}
		return budgetCurrency, nil
	}
	digits := map[string]int{}
	digitsOf := func(currencyID string) (int, error) {
		if d, ok := digits[currencyID]; ok {
			return d, nil
		}
		d, err := s.steps.Currencies.FractionDigits(ctx, currencyID)
		if err == nil {
			digits[currencyID] = d
		}
		return d, err
	}

	prompt := debtFacts{Timezone: facts.Timezone, Language: facts.Language, Categories: []debtCategory{}}
	candidates := map[string]debtCandidate{}
	for _, c := range own {
		key := strings.ToLower(c.Id)
		if c.IsArchived == 1 || c.Type != model.TypeExpense.Alias() || covered[key] {
			continue
		}
		h := byID[key]
		var payment vo.DecimalNumber
		if planned != nil {
			payment = planned[key]
		} else {
			payment = medianPaid(h.Amounts)
		}
		if !positive(payment) {
			continue
		}
		currency, err := currencyOf(key)
		if err != nil {
			return err
		}
		fd, err := digitsOf(currency)
		if err != nil {
			slog.DebugContext(ctx, "onboarding debt skipped", "user_id", userID.String(), "category_id", c.Id)
			continue
		}
		if payment = payment.Round(fd); !positive(payment) {
			continue
		}
		item := debtCategory{ID: c.Id, Name: c.Name}
		if slices.ContainsFunc(h.Amounts, func(a string) bool { return a != "" }) {
			for _, a := range h.Amounts {
				item.Monthly = append(item.Monthly, vo.NewDecimal(orZero(a)).Round(fd).String())
			}
		}
		prompt.Categories = append(prompt.Categories, item)
		candidates[key] = debtCandidate{id: vo.MustParseId(c.Id), name: c.Name, currency: vo.MustParseId(currency), payment: payment}
	}
	if len(candidates) == 0 {
		return nil
	}
	body, err := json.Marshal(prompt)
	if err != nil {
		return err
	}
	reply, err := s.completer.Complete(ctx, userID.String(), debtsSystem, string(body))
	if err != nil {
		return err
	}
	found, ok := parseDebts(reply)
	if !ok {
		return MalformedReplyError{}
	}
	var firstErr error
	created := 0
	seen := map[string]bool{}
	for _, f := range found {
		key := strings.ToLower(strings.TrimSpace(f.CategoryID))
		c, asked := candidates[key]
		if !asked || seen[key] {
			continue
		}
		seen[key] = true
		name := strings.TrimSpace(f.Name)
		if name == "" || utf8.RuneCountInString(name) > debtNameMax {
			name = c.name
		}
		ok, err := s.debts.CreateDebt(ctx, userID, budgetID, name, c.id, c.currency, c.payment, model.RecurringScheduleMonthly)
		switch {
		case err != nil:
			if firstErr == nil {
				firstErr = err
			}
		case ok:
			created++
		default:
			slog.DebugContext(ctx, "onboarding debt skipped", "user_id", userID.String(), "category_id", c.id.String())
		}
	}
	if created > 0 {
		slog.InfoContext(ctx, "onboarding debts created", "user_id", userID.String(), "count", created)
	}
	return firstErr
}

type foundDebt struct {
	CategoryID string `json:"categoryId"`
	Name       string `json:"name"`
}

// parseDebts reads {"debts":[...]} out of the reply, tolerating a code fence
// or prose around it.
func parseDebts(reply string) ([]foundDebt, bool) {
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var parsed struct {
		Debts []foundDebt `json:"debts"`
	}
	if err := json.Unmarshal([]byte(reply[start:end+1]), &parsed); err != nil {
		return nil, false
	}
	return parsed.Debts, true
}

// medianPaid is the median of the months with spending above zero; zero when
// fewer than debtMinPaidMonths have any.
func medianPaid(amounts []string) vo.DecimalNumber {
	var paid []vo.DecimalNumber
	for _, a := range amounts {
		if d := vo.NewDecimal(orZero(a)); positive(d) {
			paid = append(paid, d)
		}
	}
	if len(paid) < debtMinPaidMonths {
		return vo.NewDecimal("0")
	}
	slices.SortFunc(paid, func(a, b vo.DecimalNumber) int {
		switch {
		case a.IsLessThan(b):
			return -1
		case a.IsGreaterThan(b):
			return 1
		}
		return 0
	})
	mid := len(paid) / 2
	if len(paid)%2 == 1 {
		return paid[mid]
	}
	return paid[mid-1].Add(paid[mid]).Div(vo.NewDecimal("2"))
}

func positive(d vo.DecimalNumber) bool { return !d.IsZero() && !d.IsNegative() }

func anyPositive(amounts map[string]vo.DecimalNumber) bool {
	for _, a := range amounts {
		if positive(a) {
			return true
		}
	}
	return false
}

func orZero(s string) string {
	if strings.TrimSpace(s) == "" {
		return "0"
	}
	return s
}
