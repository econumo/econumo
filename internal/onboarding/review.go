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
// background requests pick icons for the user's categories and accounts, flag
// the savings accounts in the budget this call created and turn the categories
// that repay a loan into debts.
func (s *Service) FinishImport(ctx context.Context, userID vo.Id, req model.OnboardingFinishImportRequest) (*model.OnboardingFinishImportResult, error) {
	out := &model.OnboardingFinishImportResult{}
	var review bool
	var savingsBudget, debtBudget *vo.Id
	err := s.steps.Tx.WithTx(ctx, func(ctx context.Context) error {
		review, savingsBudget, debtBudget = false, nil, nil
		user, proceed, err := s.begin(ctx, userID, req.Id, StepImport)
		if err != nil {
			return err
		}
		budgetID, writable, found, err := s.steps.Budgets.DefaultBudget(ctx, userID, preferredBudget(user))
		if err != nil {
			return err
		}
		if found {
			out.BudgetId = budgetID.String()
			if writable {
				debtBudget = &budgetID
			}
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
				savingsBudget, debtBudget = &id, &id
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
		s.startReview(ctx, userID, savingsBudget, debtBudget)
	}
	return out, nil
}

// The import review is four independent requests, so a slow or failing one
// (the icon replies take real providers 20-60 s) never holds back the others.
// The manual steps' reviews are split the same way.
var (
	reviewCategories = withPart(jobReview, "categories")
	reviewAccounts   = withPart(jobReview, "accounts")
	reviewSavings    = withPart(jobReview, "savings")
	reviewDebts      = withPart(jobReview, "debts")
	stepCategories   = withPart(jobStepReview, "categories")
	stepAccounts     = withPart(jobStepReview, "accounts")
	stepSavings      = withPart(jobClassify, "savings")
)

func withPart(spec jobSpec, part string) jobSpec {
	spec.part = part
	return spec
}

// startReview starts the import review's requests: category icons, account
// icons, savings (when this call created the budget) and debts (when the user
// may plan the budget). Together they count once against the per-user cap.
func (s *Service) startReview(ctx context.Context, userID vo.Id, savingsBudget, debtBudget *vo.Id) {
	if !s.reserveJob(ctx, userID, jobReview) {
		return
	}
	facts := s.promptContext(ctx, userID)
	s.runJob(userID, reviewCategories, facts, func(jobCtx context.Context, facts promptFacts) error {
		return s.reviewCategoryIcons(jobCtx, userID, facts, nil)
	})
	s.runJob(userID, reviewAccounts, facts, func(jobCtx context.Context, facts promptFacts) error {
		return s.reviewAccountIcons(jobCtx, userID, facts, nil)
	})
	if savingsBudget != nil {
		budgetID := *savingsBudget
		s.runJob(userID, reviewSavings, facts, func(jobCtx context.Context, facts promptFacts) error {
			return s.reviewSavings(jobCtx, userID, budgetID, facts)
		})
	}
	if debtBudget != nil && s.debts != nil {
		budgetID := *debtBudget
		s.runJob(userID, reviewDebts, facts, func(jobCtx context.Context, facts promptFacts) error {
			return s.detectDebts(jobCtx, userID, budgetID, facts, nil)
		})
	}
}

// startCategoryReview starts save-categories' request: icons for the user's
// categories still on a default icon, the rows the step created on an icon
// nobody chose (auto, id -> icon) included.
func (s *Service) startCategoryReview(ctx context.Context, userID vo.Id, auto map[string]string) {
	if !s.reserveJob(ctx, userID, jobStepReview) {
		return
	}
	s.runJob(userID, stepCategories, s.promptContext(ctx, userID), func(jobCtx context.Context, facts promptFacts) error {
		return s.reviewCategoryIcons(jobCtx, userID, facts, auto)
	})
}

// startAccountsReview starts save-accounts' requests: icons for the user's
// accounts still on a default icon (auto as for categories) and, when the step
// put new accounts in a budget, which of them are savings. Together they count
// once against the per-user cap.
func (s *Service) startAccountsReview(ctx context.Context, userID vo.Id, auto map[string]string, budgetID vo.Id, savings []vo.Id) {
	if !s.reserveJob(ctx, userID, jobStepReview) {
		return
	}
	facts := s.promptContext(ctx, userID)
	s.runJob(userID, stepAccounts, facts, func(jobCtx context.Context, facts promptFacts) error {
		return s.reviewAccountIcons(jobCtx, userID, facts, auto)
	})
	if len(savings) > 0 {
		s.runJob(userID, stepSavings, facts, func(jobCtx context.Context, facts promptFacts) error {
			return s.classify(jobCtx, userID, budgetID, savings, facts)
		})
	}
}

var iconRules = `Names may be in any language; use the user's timezone and language to read local bank and product names. ` +
	`Use only icons from this list: ` + strings.Join(Icons, ", ") + `. `

var categoryIconsSystem = `You help set up a personal budget from the user's bank data and the names they typed. ` +
	`For each category pick the icon that best shows what the money is spent on or comes from. ` + iconRules +
	`Reply with JSON only, exactly {"categories":{"<id>":"<icon>"}}; leave out a category when no icon fits.`

var accountIconsSystem = `You help set up a personal budget from the user's bank data and the names they typed. ` +
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

// reviewCategoryIcons asks for an icon per active own category still on a
// default icon, or on the icon auto names for it, and applies each one in the
// catalogue, only while the category still has the icon it had when the job
// read it (a change the user made since wins). An unusable reply changes
// nothing.
func (s *Service) reviewCategoryIcons(ctx context.Context, userID vo.Id, facts promptFacts, auto map[string]string) error {
	categories, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		return err
	}
	prompt := categoryIconFacts{Timezone: facts.Timezone, Language: facts.Language, Categories: []reviewCategory{}}
	from := map[string]string{}
	for _, c := range categories {
		if c.IsArchived == 0 && (repaintableCategory[c.Icon] || unchosen(auto, c.Id, c.Icon)) {
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

// reviewAccountIcons does the same for live own accounts still on the
// import's icon or on the icon auto names for them.
func (s *Service) reviewAccountIcons(ctx context.Context, userID vo.Id, facts promptFacts, auto map[string]string) error {
	accounts, err := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
	if err != nil {
		return err
	}
	prompt := promptFacts{Timezone: facts.Timezone, Language: facts.Language, Accounts: []promptAccount{}}
	from := map[string]string{}
	for _, a := range accounts {
		if repaintableAccount[a.Icon] || unchosen(auto, a.ID, a.Icon) {
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

// unchosen reports whether the item still shows the icon a step created it
// with when nobody chose one.
func unchosen(auto map[string]string, id, icon string) bool {
	created, ok := auto[strings.ToLower(id)]
	return ok && created == icon
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
