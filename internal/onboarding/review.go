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
// a background request flags the savings accounts in the budget this call
// created. The icons of what the import created were already asked for by the
// import itself (ReviewImportedIcons), so this call asks for none.
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
	if review && savingsBudget != nil {
		budgetID := *savingsBudget
		s.startJob(ctx, userID, reviewSavings, func(jobCtx context.Context, facts promptFacts) error {
			return s.reviewSavings(jobCtx, userID, budgetID, facts)
		})
	}
	return out, nil
}

// The import review is three independent requests, so a slow or failing one
// (the icon replies take real providers 20-60 s) never holds back the others.
var (
	reviewCategories = withPart(jobReview, "categories")
	reviewAccounts   = withPart(jobReview, "accounts")
	reviewSavings    = withPart(jobReview, "savings")
)

func withPart(spec jobSpec, part string) jobSpec {
	spec.part = part
	return spec
}

// ReviewImportedIcons asks, in the background, for icons for the categories
// and accounts a CSV import created: two independent requests (a slow or
// failing one never holds back the other) that together count once against
// the per-user cap. Only the caller's own items still on a default icon are
// asked about.
func (s *Service) ReviewImportedIcons(ctx context.Context, userID vo.Id, categoryIDs, accountIDs []vo.Id) {
	if len(categoryIDs) == 0 && len(accountIDs) == 0 {
		return
	}
	if !s.reserveJob(ctx, userID, jobReview) {
		return
	}
	facts := s.promptContext(ctx, userID)
	if len(categoryIDs) > 0 {
		only := idSet(categoryIDs)
		s.runJob(userID, reviewCategories, facts, func(jobCtx context.Context, facts promptFacts) error {
			return s.reviewCategoryIcons(jobCtx, userID, facts, only)
		})
	}
	if len(accountIDs) > 0 {
		only := idSet(accountIDs)
		s.runJob(userID, reviewAccounts, facts, func(jobCtx context.Context, facts promptFacts) error {
			return s.reviewAccountIcons(jobCtx, userID, facts, only)
		})
	}
}

func idSet(ids []vo.Id) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[strings.ToLower(id.String())] = true
	}
	return out
}

var iconRules = `Names may be in any language; use the user's timezone and language to read local bank and product names. ` +
	`Use only icons from this list: ` + strings.Join(Icons, ", ") + `. `

var categoryIconsSystem = `You help set up a personal budget from the user's imported bank data. ` +
	`For each category pick the icon that best shows what the money is spent on or comes from. ` + iconRules +
	`Reply with JSON only, exactly {"categories":{"<id>":"<icon>"}}; leave out a category when no icon fits.`

var accountIconsSystem = `You help set up a personal budget from the user's imported bank data. ` +
	`For each account pick the icon that best shows what kind of account it is (a card, a bank account, cash, savings, an investment). ` + iconRules +
	`Reply with JSON only, exactly {"accounts":{"<id>":"<icon>"}}; leave out an account when no icon fits.`

type reviewCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type categoryIconFacts struct {
	Timezone   string           `json:"timezone"`
	Language   string           `json:"language"`
	Categories []reviewCategory `json:"categories"`
}

// repaintable are the icons an item still has when nobody chose one: the
// create-time default and the icons the CSV import gives what it creates. Only
// such items get an AI icon; an icon the user picked is never replaced.
var (
	repaintableCategory = map[string]bool{model.DefaultIcon: true, model.ImportCategoryIcon: true}
	repaintableAccount  = map[string]bool{model.ImportAccountIcon: true}
)

// reviewCategoryIcons asks for an icon per active own category in only still
// on a default icon and applies each one in the catalogue, only while the category
// still has the icon it had when the job read it (a change the user made since
// wins). An unusable reply changes nothing.
func (s *Service) reviewCategoryIcons(ctx context.Context, userID vo.Id, facts promptFacts, only map[string]bool) error {
	categories, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		return err
	}
	prompt := categoryIconFacts{Timezone: facts.Timezone, Language: facts.Language, Categories: []reviewCategory{}}
	from := map[string]string{}
	for _, c := range categories {
		if only[strings.ToLower(c.Id)] && c.IsArchived == 0 && repaintableCategory[c.Icon] {
			prompt.Categories = append(prompt.Categories, reviewCategory{ID: c.Id, Name: c.Name, Type: c.Type})
			from[strings.ToLower(c.Id)] = c.Icon
		}
	}
	if len(prompt.Categories) == 0 {
		return nil
	}
	icons, err := s.askIcons(ctx, userID, categoryIconsSystem, prompt, "categories")
	if err != nil {
		return err
	}
	return s.applyIcons(icons, from, func(id vo.Id, from, to string) error {
		_, err := s.steps.Categories.ReplaceIcon(ctx, userID, id, from, to)
		return err
	})
}

// reviewAccountIcons does the same for live own accounts in only still on the
// import's icon.
func (s *Service) reviewAccountIcons(ctx context.Context, userID vo.Id, facts promptFacts, only map[string]bool) error {
	accounts, err := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
	if err != nil {
		return err
	}
	prompt := promptFacts{Timezone: facts.Timezone, Language: facts.Language, Accounts: []promptAccount{}}
	from := map[string]string{}
	for _, a := range accounts {
		if only[strings.ToLower(a.ID)] && repaintableAccount[a.Icon] {
			prompt.Accounts = append(prompt.Accounts, promptAccount{ID: a.ID, Name: a.Name, Currency: a.CurrencyCode})
			from[strings.ToLower(a.ID)] = a.Icon
		}
	}
	if len(prompt.Accounts) == 0 {
		return nil
	}
	icons, err := s.askIcons(ctx, userID, accountIconsSystem, prompt, "accounts")
	if err != nil {
		return err
	}
	return s.applyIcons(icons, from, func(id vo.Id, from, to string) error {
		_, err := s.steps.Accounts.ReplaceIcon(ctx, userID, id, from, to)
		return err
	})
}

// reviewSavings asks which live own accounts are savings, with the
// save-accounts job's prompt, and flags them in the budget the import created.
func (s *Service) reviewSavings(ctx context.Context, userID, budgetID vo.Id, facts promptFacts) error {
	accounts, err := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
	if err != nil {
		return err
	}
	ids := make([]vo.Id, 0, len(accounts))
	for _, a := range accounts {
		ids = append(ids, vo.MustParseId(a.ID))
	}
	return s.classify(ctx, userID, budgetID, ids, facts)
}

// askIcons sends one icon request and reads the {key:{id:icon}} map out of the
// reply (tolerating a code fence or prose around it), ids lower-cased.
func (s *Service) askIcons(ctx context.Context, userID vo.Id, system string, prompt any, key string) (map[string]string, error) {
	body, err := json.Marshal(prompt)
	if err != nil {
		return nil, err
	}
	if !s.mayAsk(ctx, userID) {
		return nil, nil
	}
	raw, err := s.completer.Complete(ctx, userID.String(), system, string(body))
	if err != nil {
		return nil, err
	}
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, MalformedReplyError{}
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return nil, MalformedReplyError{}
	}
	var icons map[string]string
	if err := json.Unmarshal(parsed[key], &icons); err != nil && parsed[key] != nil {
		return nil, MalformedReplyError{}
	}
	out := make(map[string]string, len(icons))
	for id, icon := range icons {
		out[strings.ToLower(strings.TrimSpace(id))] = strings.TrimSpace(icon)
	}
	return out, nil
}

// applyIcons replaces each asked-about item's icon with a catalogue icon from
// the reply; the first error is returned after trying them all.
func (s *Service) applyIcons(icons, from map[string]string, replace func(id vo.Id, from, to string) error) error {
	var firstErr error
	for id, icon := range icons {
		old, asked := from[id]
		if !asked || !iconSet[icon] || icon == old {
			continue
		}
		if err := replace(vo.MustParseId(id), old, icon); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
