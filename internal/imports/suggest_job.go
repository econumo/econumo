package imports

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	suggestTimeout = 60 * time.Second
	// A row posted longer than this before it was imported is a backfill, not
	// a live import: the user has had the chance to classify it already.
	suggestMaxAge = 7 * 24 * time.Hour
	// The posted time is a wall clock in the owner's zone, which is not known
	// until the job runs: the first cut allows any zone's offset, and the
	// owner's own zone decides (liveImport).
	maxZoneOffset  = 14 * time.Hour
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
		if l.TransactionID != nil && l.ImportedAt.Sub(l.ExternalPostedAt) <= suggestMaxAge+maxZoneOffset {
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
		if err := s.suggest(live); err != nil {
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
// on its own: one owner's failure does not stop the others. Each owner gets
// its own timeout, started once its lock is held, so waiting for another job
// of the same owner does not eat into it.
func (s *Service) suggest(links []model.ImportTransactionLink) error {
	ctx, cancel := context.WithTimeout(context.Background(), suggestTimeout)
	defer cancel()
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
		s.suggestForOwnerSafe(owner, byOwner[owner])
	}
	return nil
}

// liveImport: a row posted at most suggestMaxAge before it was imported, both
// read as wall clocks in the owner's zone (UTC when unknown or invalid).
func liveImport(l model.ImportTransactionLink, timezone string) bool {
	loc, err := time.LoadLocation(timezone)
	if timezone == "" || err != nil {
		loc = time.UTC
	}
	return wallClock(l.ImportedAt.In(loc)).Sub(l.ExternalPostedAt) <= suggestMaxAge
}

// suggestForOwnerSafe keeps one owner's error or panic from reaching the
// others. Never the error text, the prompt or the reply: they can carry names.
func (s *Service) suggestForOwnerSafe(owner vo.Id, candidates []suggestCandidate) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("import suggest failed", "user_id", owner.String(), "err_type", fmt.Sprintf("%T", r))
		}
	}()
	unlock := s.suggestLocks.lock(owner)
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), suggestTimeout)
	defer cancel()
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
	fresh := candidates[:0:0]
	for _, c := range candidates {
		if liveImport(c.link, timezone) {
			fresh = append(fresh, c)
		}
	}
	if candidates = fresh; len(candidates) == 0 {
		return nil
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
	open = vocab.liveOpen(open)
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
		var gone []vo.Id
		err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			var err error
			gone, err = s.addSuggestionLinks(ctx, covered)
			return err
		})
		if err != nil {
			return err
		}
		// A suggestion the list read deleted meanwhile covers nothing: its rows
		// go to the AI like any other.
		goneLinks := map[vo.Id]bool{}
		for _, id := range gone {
			for _, l := range covered[id] {
				goneLinks[l] = true
			}
		}
		for _, r := range rows {
			if goneLinks[r.Link.ID] {
				rest = append(rest, r)
			}
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
	groups, groupMissing := groupRowsMissing(valid)
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		// Re-read: the call took seconds, and the user may have settled
		// suggestions meanwhile.
		open, err := s.repo.ListOpenSuggestionsByUser(ctx, owner)
		if err != nil {
			return err
		}
		rest, parts := mergeIntoOpen(groups, groupMissing, vocab.liveOpen(open), vocab.income)
		joins := make(map[vo.Id][]vo.Id, len(parts))
		for id, ps := range parts {
			for _, p := range ps {
				joins[id] = append(joins[id], p.links...)
			}
		}
		gone, err := s.addSuggestionLinks(ctx, joins)
		if err != nil {
			return err
		}
		// A suggestion deleted between the re-read and the join: its rows open
		// suggestions under their own group's classification.
		for _, id := range gone {
			for _, p := range parts[id] {
				rest[p.group] = append(rest[p.group], p.links...)
			}
		}
		now := s.clk.Now().UTC()
		for i, links := range rest {
			if len(links) == 0 {
				continue
			}
			g := groups[i]
			g.LinkIDs = links
			g.UserID, g.CreatedAt, g.UpdatedAt = owner, now, now
			if err := s.repo.InsertSuggestion(ctx, &g); err != nil {
				return err
			}
		}
		return nil
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
// concurrent writers, and returns the suggestions that were no longer open.
func (s *Service) addSuggestionLinks(ctx context.Context, joins map[vo.Id][]vo.Id) ([]vo.Id, error) {
	ids := make([]vo.Id, 0, len(joins))
	for id := range joins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	now := s.clk.Now().UTC()
	var gone []vo.Id
	for _, id := range ids {
		ok, err := s.repo.AddSuggestionLinks(ctx, id, joins[id], now)
		if err != nil {
			return nil, err
		}
		if !ok {
			gone = append(gone, id)
		}
	}
	return gone, nil
}

type ownerLocks struct {
	mu sync.Mutex
	m  map[vo.Id]*ownerLock
}

type ownerLock struct {
	mu   sync.Mutex
	refs int
}

// lock holds the owner's lock until the returned func runs; an entry lives
// only while someone holds or waits for it.
func (l *ownerLocks) lock(owner vo.Id) func() {
	l.mu.Lock()
	if l.m == nil {
		l.m = map[vo.Id]*ownerLock{}
	}
	e := l.m[owner]
	if e == nil {
		e = &ownerLock{}
		l.m[owner] = e
	}
	e.refs++
	l.mu.Unlock()
	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		l.mu.Lock()
		if e.refs--; e.refs == 0 {
			delete(l.m, owner)
		}
		l.mu.Unlock()
	}
}
