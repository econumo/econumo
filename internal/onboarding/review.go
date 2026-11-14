package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

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

// The import review is independent requests, so a slow or failing one (the
// icon replies take real providers 20-60 s) never holds back the others.
var (
	reviewCategories = withPart(jobIcons, "categories")
	reviewAccounts   = withPart(jobIcons, "accounts")
	reviewSavings    = withPart(jobReview, "savings")
)

func withPart(spec jobSpec, part string) jobSpec {
	spec.part = part
	return spec
}

// pendingIcons collects what a user's CSV import chunks created until the
// import goes quiet; gen tells the timer of the latest chunk from older ones.
type pendingIcons struct {
	categories, accounts []vo.Id
	seen                 map[string]bool
	facts                promptFacts
	gen                  int
	stop                 func() bool
	retried              bool
}

// ReviewImportedIcons notes the categories and accounts a CSV import request
// created; it is called for every successful request, empty lists included. A
// large file arrives in several requests, so the review starts once no further
// request from the user has come for iconReviewQuiet (one that created nothing
// keeps a pending review waiting and starts none): two
// independent background requests (categories, accounts) over everything the
// chunks created, counted once against the icon cap. Only the caller's own
// items still on a default icon are asked about. The state is in memory: a
// restart in the quiet period drops that review.
func (s *Service) ReviewImportedIcons(ctx context.Context, userID vo.Id, categoryIDs, accountIDs []vo.Id) {
	if s.completer == nil {
		return
	}
	created := len(categoryIDs) > 0 || len(accountIDs) > 0
	var facts promptFacts
	if created {
		facts = s.promptContext(ctx, userID)
	}
	key := userID.String()
	s.iconsMu.Lock()
	defer s.iconsMu.Unlock()
	p := s.pendingIcons[key]
	if p == nil && !created {
		// nothing pending: a chunk that created nothing starts no review
		return
	}
	if p == nil {
		p = &pendingIcons{seen: map[string]bool{}}
		s.pendingIcons[key] = p
	}
	for _, id := range categoryIDs {
		if k := "c" + strings.ToLower(id.String()); !p.seen[k] {
			p.seen[k] = true
			p.categories = append(p.categories, id)
		}
	}
	for _, id := range accountIDs {
		if k := "a" + strings.ToLower(id.String()); !p.seen[k] {
			p.seen[k] = true
			p.accounts = append(p.accounts, id)
		}
	}
	if created {
		p.facts = facts
	}
	if s.iconInflight[key] == 0 {
		s.rearmIconReview(userID, p)
	}
}

// rearmIconReview restarts the quiet period of a pending review; the caller
// holds iconsMu.
func (s *Service) rearmIconReview(userID vo.Id, p *pendingIcons) {
	p.gen++
	if p.stop != nil {
		p.stop()
	}
	gen := p.gen
	p.stop = s.schedule(iconReviewQuiet, func() { s.startIconReview(userID, gen) })
}

// ImportStarted holds a pending review while a CSV import request is in
// flight (a slow chunk must not let the quiet period of the previous one run
// out mid-import). The returned release must be called when the request ends,
// success or failure; it restarts the quiet period when ids are pending.
func (s *Service) ImportStarted(userID vo.Id) (release func()) {
	if s.completer == nil {
		return func() {}
	}
	key := userID.String()
	s.iconsMu.Lock()
	s.iconInflight[key]++
	if p := s.pendingIcons[key]; p != nil {
		p.gen++
		if p.stop != nil {
			p.stop()
		}
	}
	s.iconsMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.iconsMu.Lock()
			defer s.iconsMu.Unlock()
			if s.iconInflight[key]--; s.iconInflight[key] <= 0 {
				delete(s.iconInflight, key)
				if p := s.pendingIcons[key]; p != nil {
					s.rearmIconReview(userID, p)
				}
			}
		})
	}
}

// startIconReview runs the review a quiet period ended, unless a later chunk
// has rescheduled it.
func (s *Service) startIconReview(userID vo.Id, gen int) {
	key := userID.String()
	s.iconsMu.Lock()
	p := s.pendingIcons[key]
	if p == nil || p.gen != gen {
		s.iconsMu.Unlock()
		return
	}
	delete(s.pendingIcons, key)
	s.iconsMu.Unlock()

	ok, err := s.reserve(context.Background(), userID, jobIcons)
	if err != nil {
		s.retryIconReview(userID, p, err)
		return
	}
	if !ok {
		return
	}
	if len(p.categories) > 0 {
		only := idSet(p.categories)
		s.runJob(userID, reviewCategories, p.facts, func(jobCtx context.Context, facts promptFacts) error {
			return s.reviewCategoryIcons(jobCtx, userID, facts, only)
		})
	}
	if len(p.accounts) > 0 {
		only := idSet(p.accounts)
		s.runJob(userID, reviewAccounts, p.facts, func(jobCtx context.Context, facts promptFacts) error {
			return s.reviewAccountIcons(jobCtx, userID, facts, only)
		})
	}
}

// retryIconReview puts a review back after it could not find out whether it
// may run (an opt-out read or the limiter failed), once: a second failure
// drops it. A chunk that arrived meanwhile has its own pending review, which
// takes these ids too.
func (s *Service) retryIconReview(userID vo.Id, p *pendingIcons, err error) {
	if p.retried {
		slog.Warn(jobIcons.skipped, "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		return
	}
	slog.Warn("import icon review postponed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
	key := userID.String()
	s.iconsMu.Lock()
	defer s.iconsMu.Unlock()
	if cur := s.pendingIcons[key]; cur != nil {
		for _, id := range p.categories {
			if k := "c" + strings.ToLower(id.String()); !cur.seen[k] {
				cur.seen[k] = true
				cur.categories = append(cur.categories, id)
			}
		}
		for _, id := range p.accounts {
			if k := "a" + strings.ToLower(id.String()); !cur.seen[k] {
				cur.seen[k] = true
				cur.accounts = append(cur.accounts, id)
			}
		}
		return
	}
	p.retried = true
	p.gen++
	gen := p.gen
	s.pendingIcons[key] = p
	p.stop = s.schedule(iconReviewQuiet, func() { s.startIconReview(userID, gen) })
}

func idSet(ids []vo.Id) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[strings.ToLower(id.String())] = true
	}
	return out
}

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
