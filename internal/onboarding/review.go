package onboarding

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// StepImport is the import step's name in the onboarding option.
const StepImport = "import"

// FinishImport closes the import step after a successful CSV import: in one
// transaction it creates the budget when the user has none (over every live own
// account outside a hidden folder, when there is one) and marks the import step
// done. The categories and accounts steps stay open: they become reviews of
// what the import created. After the commit, when an AI provider is configured,
// a background job picks icons for the user's categories and accounts and flags
// the savings accounts in the budget this call created.
func (s *Service) FinishImport(ctx context.Context, userID vo.Id, req model.OnboardingFinishImportRequest) (*model.OnboardingFinishImportResult, error) {
	out := &model.OnboardingFinishImportResult{}
	var review bool
	var savingsBudget *vo.Id
	err := s.steps.Tx.WithTx(ctx, func(ctx context.Context) error {
		review, savingsBudget = false, nil
		user, proceed, err := s.begin(ctx, userID, req.Id, StepImport)
		if err != nil {
			return err
		}
		if out.BudgetId, err = s.defaultBudgetID(ctx, userID, user); err != nil {
			return err
		}
		if !proceed {
			out.User = *user
			return nil
		}
		if out.BudgetId == "" {
			accounts, aerr := s.ownVisibleAccountIDs(ctx, userID)
			if aerr != nil {
				return aerr
			}
			if len(accounts) > 0 {
				if out.BudgetId, err = s.createBudget(ctx, userID, accounts); err != nil {
					return err
				}
				id := vo.MustParseId(out.BudgetId)
				savingsBudget = &id
			}
		}
		done, err := s.finish(ctx, userID, req.Id, StepImport)
		if err != nil {
			return err
		}
		out.User = *done
		review = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if review {
		s.startJob(ctx, userID, jobReview, func(jobCtx context.Context, facts promptFacts) error {
			return s.review(jobCtx, userID, savingsBudget, facts)
		})
	}
	return out, nil
}

var reviewSystem = `You help set up a personal budget from the user's imported bank data. ` +
	`For each category pick the icon that best shows what the money is spent on or comes from; ` +
	`for each account pick the icon that best shows what kind of account it is (a card, a bank account, cash, savings, an investment). ` +
	`Also decide which accounts are savings accounts: a savings account holds money the user is putting aside (a savings or deposit account, an emergency fund, a piggy bank, an investment or brokerage account kept for the future); everyday accounts are for spending (cards, checking, cash, wallets). ` +
	`Names may be in any language; use the user's timezone and language to read local bank and product names. ` +
	`Use only icons from this list: ` + strings.Join(Icons, ", ") + `. ` +
	`Reply with JSON only, exactly {"categories":{"<id>":"<icon>"},"accounts":{"<id>":"<icon>"},"savings":["<id>", ...]}; ` +
	`leave out an item when no icon fits, and use an empty savings list when no account is a savings account.`

type reviewCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type reviewFacts struct {
	Timezone   string           `json:"timezone"`
	Language   string           `json:"language"`
	Categories []reviewCategory `json:"categories"`
	Accounts   []promptAccount  `json:"accounts"`
}

type reviewReply struct {
	Categories map[string]string `json:"categories"`
	Accounts   map[string]string `json:"accounts"`
	Savings    []string          `json:"savings"`
}

// repaintable are the icons an item still has when nobody chose one: the
// create-time default and the icons the CSV import gives what it creates. Only
// such items get an AI icon; an icon the user picked is never replaced.
var (
	repaintableCategory = map[string]bool{model.DefaultIcon: true, model.ImportCategoryIcon: true}
	repaintableAccount  = map[string]bool{model.ImportAccountIcon: true}
)

// review asks the AI for an icon per active own category and live own account
// still on a default icon, and which accounts are savings (every live own
// account is asked about), then applies the answer: an icon only when it is in
// the catalogue and the item still has the icon it had when the job read it (a
// change the user made since wins), savings only in savingsBudget and only for
// accounts that are still members. An unusable reply changes nothing.
func (s *Service) review(ctx context.Context, userID vo.Id, savingsBudget *vo.Id, facts promptFacts) error {
	categories, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		return err
	}
	accounts, err := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
	if err != nil {
		return err
	}
	prompt := reviewFacts{Timezone: facts.Timezone, Language: facts.Language, Categories: []reviewCategory{}, Accounts: []promptAccount{}}
	catIcon := map[string]string{}
	for _, c := range categories {
		if c.IsArchived == 0 && repaintableCategory[c.Icon] {
			prompt.Categories = append(prompt.Categories, reviewCategory{ID: c.Id, Name: c.Name, Type: c.Type})
			catIcon[strings.ToLower(c.Id)] = c.Icon
		}
	}
	accIcon := map[string]string{}
	repaint := map[string]bool{}
	for _, a := range accounts {
		prompt.Accounts = append(prompt.Accounts, promptAccount{ID: a.ID, Name: a.Name, Currency: a.CurrencyCode})
		accIcon[strings.ToLower(a.ID)] = a.Icon
		if repaintableAccount[a.Icon] {
			repaint[strings.ToLower(a.ID)] = true
		}
	}
	if len(prompt.Categories) == 0 && len(prompt.Accounts) == 0 {
		return nil
	}
	body, err := json.Marshal(prompt)
	if err != nil {
		return err
	}
	raw, err := s.completer.Complete(ctx, userID.String(), reviewSystem, string(body))
	if err != nil {
		return err
	}
	reply, ok := parseReview(raw)
	if !ok {
		return MalformedReplyError{}
	}
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for id, icon := range reply.Categories {
		if from, asked := catIcon[id]; asked && iconSet[icon] && icon != from {
			_, rerr := s.steps.Categories.ReplaceIcon(ctx, userID, vo.MustParseId(id), from, icon)
			keep(rerr)
		}
	}
	for id, icon := range reply.Accounts {
		if from, asked := accIcon[id]; asked && repaint[id] && iconSet[icon] && icon != from {
			_, rerr := s.steps.Accounts.ReplaceIcon(ctx, userID, vo.MustParseId(id), from, icon)
			keep(rerr)
		}
	}
	if savingsBudget != nil {
		var picked []vo.Id
		for _, id := range reply.Savings {
			if _, asked := accIcon[id]; asked {
				picked = append(picked, vo.MustParseId(id))
			}
		}
		if len(picked) > 0 {
			keep(s.budgets.MarkSavingsMembers(ctx, userID, *savingsBudget, picked))
		}
	}
	return firstErr
}

// parseReview reads the reply's JSON object (tolerating a code fence or prose
// around it), with ids lower-cased and icons trimmed.
func parseReview(raw string) (reviewReply, bool) {
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return reviewReply{}, false
	}
	var parsed reviewReply
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return reviewReply{}, false
	}
	out := reviewReply{Categories: map[string]string{}, Accounts: map[string]string{}}
	for id, icon := range parsed.Categories {
		out.Categories[strings.ToLower(strings.TrimSpace(id))] = strings.TrimSpace(icon)
	}
	for id, icon := range parsed.Accounts {
		out.Accounts[strings.ToLower(strings.TrimSpace(id))] = strings.TrimSpace(icon)
	}
	seen := map[string]bool{}
	for _, id := range parsed.Savings {
		id = strings.ToLower(strings.TrimSpace(id))
		if !seen[id] {
			seen[id] = true
			out.Savings = append(out.Savings, id)
		}
	}
	return out, true
}
