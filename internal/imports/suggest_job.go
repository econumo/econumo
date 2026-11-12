package imports

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	suggestTimeout = 60 * time.Second
	// A row posted longer than this before it was imported is a backfill, not
	// a live import: the user has had the chance to classify it already.
	suggestMaxAge  = 7 * 24 * time.Hour
	suggestMaxRows = 50
)

// startSuggestJob asks the AI, in the background, for rules that would have
// classified the given freshly created rows. It returns at once; the import
// it follows has already committed and never depends on the job.
func (s *Service) startSuggestJob(links []model.ImportTransactionLink) {
	if s.completer == nil || s.facts == nil {
		return
	}
	var live []model.ImportTransactionLink
	for _, l := range links {
		if l.TransactionID != nil && l.ImportedAt.Sub(l.ExternalPostedAt) <= suggestMaxAge {
			live = append(live, l)
		}
	}
	if len(live) == 0 {
		return
	}
	run := s.run
	if run == nil {
		run = func(f func()) { go f() }
	}
	run(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("import suggest failed", "err_type", fmt.Sprintf("%T", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), suggestTimeout)
		defer cancel()
		if err := s.suggest(ctx, live); err != nil {
			// Never the error text, the prompt or the reply: they can carry names.
			slog.Warn("import suggest failed", "err_type", fmt.Sprintf("%T", err))
		}
	})
}

type suggestCandidate struct {
	link model.ImportTransactionLink
	tx   *model.Transaction
}

// suggest groups the rows by account owner (the owner's rules classify a
// shared account, so the owner gets the suggestions) and runs each owner
// on its own: one owner's failure does not stop the others.
func (s *Service) suggest(ctx context.Context, links []model.ImportTransactionLink) error {
	byOwner := map[vo.Id][]suggestCandidate{}
	var owners []vo.Id
	for _, l := range links {
		t, err := s.lister.GetByID(ctx, *l.TransactionID)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				continue
			}
			return err
		}
		if t.Type == model.TransactionTypeTransfer || (t.CategoryID != nil && t.PayeeID != nil) {
			continue
		}
		owner, err := s.accounts.AccountOwner(ctx, t.AccountID)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				continue
			}
			return err
		}
		if _, seen := byOwner[owner]; !seen {
			owners = append(owners, owner)
		}
		byOwner[owner] = append(byOwner[owner], suggestCandidate{link: l, tx: t})
	}
	for _, owner := range owners {
		s.suggestForOwnerSafe(ctx, owner, byOwner[owner])
	}
	return nil
}

// suggestForOwnerSafe keeps one owner's error or panic from reaching the
// others. Never the error text, the prompt or the reply: they can carry names.
func (s *Service) suggestForOwnerSafe(ctx context.Context, owner vo.Id, candidates []suggestCandidate) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("import suggest failed", "user_id", owner.String(), "err_type", fmt.Sprintf("%T", r))
		}
	}()
	if err := s.suggestForOwner(ctx, owner, candidates); err != nil {
		slog.Warn("import suggest failed", "user_id", owner.String(), "err_type", fmt.Sprintf("%T", err))
	}
}

func (s *Service) suggestForOwner(ctx context.Context, owner vo.Id, candidates []suggestCandidate) error {
	language, timezone, readonly, err := s.facts.OwnerFacts(ctx, owner)
	if err != nil {
		return err
	}
	if readonly {
		return nil
	}
	if language == "" {
		language = "en"
	}
	vocab, err := s.suggestVocabulary(ctx, owner)
	if err != nil {
		return err
	}
	rows, err := s.suggestRows(ctx, owner, candidates)
	if err != nil {
		return err
	}
	open, err := s.repo.ListOpenSuggestionsByUser(ctx, owner)
	if err != nil {
		return err
	}
	covered := map[vo.Id][]vo.Id{}
	var rest []suggestRow
	for _, r := range rows {
		if id, ok := openFor(r, open, vocab.income); ok {
			covered[id] = append(covered[id], r.Link.ID)
			continue
		}
		rest = append(rest, r)
	}
	if len(covered) > 0 {
		if err := s.tx.WithTx(ctx, func(ctx context.Context) error { return s.addSuggestionLinks(ctx, covered) }); err != nil {
			return err
		}
	}
	if len(rest) == 0 {
		return nil
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].Link.ExternalPostedAt.After(rest[j].Link.ExternalPostedAt) })
	if len(rest) > suggestMaxRows {
		rest = rest[:suggestMaxRows]
	}
	if err := s.countAttempt(ctx, RateScopeSuggest, owner); err != nil {
		slog.Info("import suggest skipped", "user_id", owner.String(), "reason", "rate_limited")
		return nil
	}
	system, user := buildSuggestPrompt(rest, vocab, language, timezone)
	reply, err := s.completer.Complete(ctx, owner.String(), system, user)
	if err != nil {
		return err
	}
	parsed, err := parseSuggestReply(reply)
	if err != nil {
		return err
	}
	valid := validateReply(rest, parsed, vocab)
	if len(valid) == 0 {
		return nil
	}
	groups, groupTypes := groupRowsTyped(valid)
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		// Re-read: the call took seconds, and another job or the user may have
		// opened or settled suggestions meanwhile.
		open, err := s.repo.ListOpenSuggestionsByUser(ctx, owner)
		if err != nil {
			return err
		}
		fresh, joins := mergeIntoOpen(groups, groupTypes, open, vocab.income)
		now := s.clk.Now().UTC()
		for i := range fresh {
			fresh[i].UserID, fresh[i].CreatedAt, fresh[i].UpdatedAt = owner, now, now
			if err := s.repo.InsertSuggestion(ctx, &fresh[i]); err != nil {
				return err
			}
		}
		return s.addSuggestionLinks(ctx, joins)
	})
}

// suggestVocabulary is the owner's non-archived categories and payees, payees
// by name so the prompt's cap keeps a stable, predictable set.
func (s *Service) suggestVocabulary(ctx context.Context, owner vo.Id) (suggestVocabulary, error) {
	categories, err := s.entities.CategoriesByOwner(ctx, owner)
	if err != nil {
		return suggestVocabulary{}, err
	}
	payees, err := s.entities.PayeesByOwner(ctx, owner)
	if err != nil {
		return suggestVocabulary{}, err
	}
	categories, payees = activeNamed(categories), activeNamed(payees)
	sort.SliceStable(payees, func(i, j int) bool { return strings.ToLower(payees[i].Name) < strings.ToLower(payees[j].Name) })
	return newSuggestVocabulary(categories, payees), nil
}

func activeNamed(list []model.ImportNamed) []model.ImportNamed {
	out := make([]model.ImportNamed, 0, len(list))
	for _, n := range list {
		if !n.Archived {
			out = append(out, n)
		}
	}
	return out
}

func (s *Service) suggestRows(ctx context.Context, owner vo.Id, candidates []suggestCandidate) ([]suggestRow, error) {
	accounts, err := s.accounts.AccountListForUser(ctx, owner)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(accounts))
	for _, a := range accounts {
		names[a.Id] = a.Name
	}
	codes := map[vo.Id]string{}
	rows := make([]suggestRow, 0, len(candidates))
	for _, c := range candidates {
		code, ok := codes[c.tx.AccountID]
		if !ok {
			code, err = s.accounts.AccountCurrencyCode(ctx, c.tx.AccountID)
			if err != nil {
				return nil, err
			}
			codes[c.tx.AccountID] = code
		}
		r := suggestRow{Link: c.link, Type: c.tx.Type, Amount: c.tx.Amount, Currency: code, Account: names[c.tx.AccountID.String()]}
		r.Missing.Category, r.Missing.Payee = c.tx.CategoryID == nil, c.tx.PayeeID == nil
		rows = append(rows, r)
	}
	return rows, nil
}

// addSuggestionLinks writes the joins in id order, a stable order for
// concurrent writers.
func (s *Service) addSuggestionLinks(ctx context.Context, joins map[vo.Id][]vo.Id) error {
	ids := make([]vo.Id, 0, len(joins))
	for id := range joins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, id := range ids {
		if err := s.repo.AddSuggestionLinks(ctx, id, joins[id]); err != nil {
			return err
		}
	}
	return nil
}
