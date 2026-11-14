package imports

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	suggestTimeout = 60 * time.Second
	// The scan that sorts a run's rows by owner reads each row; a large sync
	// needs far longer than one owner's AI budget, which starts only when that
	// owner's job does.
	suggestScanTimeout = 5 * time.Minute
	suggestUndoTimeout = 10 * time.Second
	// A row posted longer than this before it was imported is a backfill, not
	// a live import: the user has had the chance to classify it already.
	suggestMaxAge  = 7 * 24 * time.Hour
	suggestMaxSkew = 24 * time.Hour
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
			slog.Warn("import suggest failed", ai.LogAttrs(err)...)
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
	ctx, cancel := context.WithTimeout(context.Background(), suggestScanTimeout)
	defer cancel()
	byOwner := map[vo.Id][]suggestCandidate{}
	var owners []vo.Id
	// A row that cannot be read is left out; the others still get their
	// suggestions.
	skip := func(l model.ImportTransactionLink, err error) {
		if _, ok := errs.AsNotFound(err); !ok {
			slog.Warn("import suggest row skipped", "link_id", l.ID.String(), "err_type", fmt.Sprintf("%T", err))
		}
	}
	for _, l := range links {
		t, err := s.lister.GetByID(ctx, *l.TransactionID)
		if err != nil {
			skip(l, err)
			continue
		}
		if t.Type == model.TransactionTypeTransfer || (t.CategoryID != nil && t.PayeeID != nil) {
			continue
		}
		owner, err := s.accounts.AccountOwner(ctx, t.AccountID)
		if err != nil {
			skip(l, err)
			continue
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

// liveImport: a row posted at most suggestMaxAge before it was imported, and
// at most suggestMaxSkew after it (clock or zone skew; a row dated further in
// the future is not a live import), both read as wall clocks in the owner's
// zone (UTC when unknown or invalid).
func liveImport(l model.ImportTransactionLink, timezone string) bool {
	loc, err := time.LoadLocation(timezone)
	if timezone == "" || err != nil {
		loc = time.UTC
	}
	age := wallClock(l.ImportedAt.In(loc)).Sub(l.ExternalPostedAt)
	return age >= -suggestMaxSkew && age <= suggestMaxAge
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
		slog.Warn("import suggest failed", append([]any{"user_id", owner.String()}, ai.LogAttrs(err)...)...)
	}
}

func (s *Service) suggestForOwner(ctx context.Context, owner vo.Id, candidates []suggestCandidate) error {
	facts, err := s.facts.OwnerFacts(ctx, owner)
	if err != nil {
		return err
	}
	if facts.Readonly || facts.AIOff {
		return nil
	}
	language, timezone := facts.Language, facts.Timezone
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
	// Every write of this run is undone when the owner turns out to have opted
	// out once it committed, the joins made before the AI call included.
	var inserted []vo.Id
	joined := map[vo.Id][]vo.Id{}
	preJoined := map[vo.Id][]vo.Id{}
	defer func() {
		all := make(map[vo.Id][]vo.Id, len(preJoined)+len(joined))
		for id, links := range preJoined {
			all[id] = append(all[id], links...)
		}
		for id, links := range joined {
			all[id] = append(all[id], links...)
		}
		s.undoIfOptedOut(ctx, owner, inserted, all)
	}()
	if len(covered) > 0 {
		var gone []vo.Id
		off := false
		err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			var err error
			gone, off = nil, false
			if off, err = s.optedOut(ctx, owner); err != nil || off {
				return err
			}
			gone, err = s.addSuggestionLinks(ctx, covered)
			return err
		})
		if err != nil || off {
			return err
		}
		// A suggestion the list read deleted meanwhile covers nothing: its rows
		// go to the AI like any other.
		goneSet := map[vo.Id]bool{}
		goneLinks := map[vo.Id]bool{}
		for _, id := range gone {
			goneSet[id] = true
			for _, l := range covered[id] {
				goneLinks[l] = true
			}
		}
		for id, links := range covered {
			if !goneSet[id] {
				preJoined[id] = links
			}
		}
		for _, r := range rows {
			if goneLinks[r.Link.ID] {
				rest = append(rest, r)
			}
		}
	}
	if rest = fillable(rest, vocab); len(rest) == 0 {
		return nil
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].Link.ExternalPostedAt.After(rest[j].Link.ExternalPostedAt) })
	if len(rest) > suggestMaxRows {
		rest = rest[:suggestMaxRows]
	}
	// The candidates took a while to gather: an opt-out meanwhile still wins.
	if off, err := s.optedOut(ctx, owner); err != nil || off {
		return err
	}
	if err := s.countAttempt(ctx, RateScopeSuggest, owner); err != nil {
		if _, limited := errs.AsTooManyRequests(err); limited {
			slog.Info("import suggest skipped", "user_id", owner.String(), "reason", "rate_limited")
			return nil
		}
		return err
	}
	parsed, err := s.askSuggestions(ctx, owner, rest, vocab, language, timezone)
	if err != nil || parsed == nil {
		return err
	}
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		inserted, joined = nil, map[vo.Id][]vo.Id{}
		// The call took seconds: a user who opted out meanwhile gets nothing
		// stored, and one who settled suggestions gets them re-read, as do
		// categories and payees archived meanwhile.
		if off, err := s.optedOut(ctx, owner); err != nil || off {
			return err
		}
		vocab, err := s.suggestVocabulary(ctx, owner)
		if err != nil {
			return err
		}
		valid := validateReply(rest, parsed, vocab)
		if len(valid) == 0 {
			return nil
		}
		groups, groupMissing := groupRowsMissing(valid)
		open, err := s.repo.ListOpenSuggestionsByUser(ctx, owner)
		if err != nil {
			return err
		}
		unjoined, parts := mergeIntoOpen(groups, groupMissing, vocab.liveOpen(open), vocab.income)
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
				unjoined[p.group] = append(unjoined[p.group], p.links...)
			}
		}
		goneSet := map[vo.Id]bool{}
		for _, id := range gone {
			goneSet[id] = true
		}
		for id, links := range joins {
			if !goneSet[id] {
				joined[id] = links
			}
		}
		now := s.clk.Now().UTC()
		for i, links := range unjoined {
			if len(links) == 0 {
				continue
			}
			g := groups[i]
			g.LinkIDs = links
			g.UserID, g.CreatedAt, g.UpdatedAt = owner, now, now
			if err := s.repo.InsertSuggestion(ctx, &g); err != nil {
				return err
			}
			inserted = append(inserted, g.ID)
		}
		return nil
	})
	if err != nil {
		// rolled back: nothing of it to undo
		inserted, joined = nil, nil
	}
	return err
}

// fillable keeps the rows a suggestion could fill: a missing payee needs some
// active payee to pick, a missing category an active category of the row's
// type. The others would cost an AI call and a cap slot for nothing.
func fillable(rows []suggestRow, vocab suggestVocabulary) []suggestRow {
	hasType := map[bool]bool{}
	for _, c := range vocab.Categories {
		hasType[c.Income] = true
	}
	out := rows[:0:0]
	for _, r := range rows {
		if (r.Missing.Payee && len(vocab.Payees) > 0) || (r.Missing.Category && hasType[r.Type.IsIncome()]) {
			out = append(out, r)
		}
	}
	return out
}

// askSuggestions runs step 1 once per transaction type in rows (that type's
// categories only), then, for the rows whose merchant name resembles some of
// the owner's payees, step 2 to pick among those. Every call after the first
// re-checks the opt-out; nil, nil means the owner opted out meanwhile.
func (s *Service) askSuggestions(ctx context.Context, owner vo.Id, rows []suggestRow, vocab suggestVocabulary, language, timezone string) (map[string]suggestReplyRow, error) {
	calls := 0
	ask := func(system, user string, schema *ai.Schema) (string, bool, error) {
		if calls > 0 {
			if off, err := s.optedOut(ctx, owner); err != nil || off {
				return "", false, err
			}
		}
		calls++
		reply, err := s.completer.Complete(ctx, owner.String(), system, user, schema)
		return reply, true, err
	}
	replies := map[string]suggestReplyRow{}
	for _, income := range []bool{false, true} {
		if !slices.ContainsFunc(rows, func(r suggestRow) bool { return r.Type.IsIncome() == income }) {
			continue
		}
		system, user := buildSuggestPrompt(rows, income, vocab, language, timezone)
		reply, ok, err := ask(system, user, suggestSchema)
		if !ok {
			return nil, err
		}
		var parsed map[string]suggestReplyRow
		if err == nil {
			parsed, err = parseSuggestReply(reply)
		}
		if err != nil {
			// the other type's answers still stand
			slog.Warn("import suggest failed", append([]any{"user_id", owner.String()}, ai.LogAttrs(err)...)...)
			continue
		}
		for k, r := range parsed {
			// only keys of the rows this request carried
			if i, err := strconv.Atoi(k); err == nil && i >= 0 && i < len(rows) && rows[i].Type.IsIncome() == income {
				replies[k] = r
			}
		}
	}
	candidates := map[string][]model.ImportNamed{}
	for k, r := range replies {
		row := rows[mustAtoi(k)]
		if !row.Missing.Payee || strings.TrimSpace(r.Merchant) == "" {
			continue
		}
		if _, _, ok := matchTextIn(row.Link, r.MatchText); !ok {
			continue
		}
		if id, ok := exactPayee(r.Merchant, r.MatchText, vocab.Payees); ok {
			r.PayeeID = id
			replies[k] = r
			continue
		}
		if list := payeeCandidates(r.Merchant, r.MatchText, vocab.Payees); len(list) > 0 {
			candidates[k] = list
		}
	}
	if len(candidates) == 0 {
		return replies, nil
	}
	system, user := buildPayeePrompt(rows, replies, candidates, language, timezone)
	reply, ok, err := ask(system, user, payeeSchema)
	if !ok {
		return nil, err
	}
	picks, perr := parsePayeeReply(reply)
	if err == nil {
		err = perr
	}
	if err != nil {
		// step 1's categories still stand without payees
		slog.Warn("import suggest payees failed", append([]any{"user_id", owner.String()}, ai.LogAttrs(err)...)...)
		return replies, nil
	}
	applyPayeeReply(replies, picks, candidates)
	return replies, nil
}

func mustAtoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// undoIfOptedOut covers an opt-out (and its suggestion cleanup) that committed
// while the write was in flight, unseen by the write's own read: what this job
// stored is removed again, best effort.
func (s *Service) undoIfOptedOut(ctx context.Context, owner vo.Id, inserted []vo.Id, joined map[vo.Id][]vo.Id) {
	if len(inserted) == 0 && len(joined) == 0 {
		return
	}
	// The owner's 60 s may be all but spent by the AI call: the undo gets a
	// short deadline of its own.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), suggestUndoTimeout)
	defer cancel()
	if off, err := s.optedOut(ctx, owner); err != nil || !off {
		return
	}
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, id := range inserted {
			if err := s.repo.DeleteSuggestion(ctx, id); err != nil {
				return err
			}
		}
		for id, links := range joined {
			if _, err := s.repo.ReleaseSuggestionLinks(ctx, id, links); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("import suggest undo failed", "user_id", owner.String(), "err_type", fmt.Sprintf("%T", err))
	}
}

func (s *Service) optedOut(ctx context.Context, owner vo.Id) (bool, error) {
	facts, err := s.facts.OwnerFacts(ctx, owner)
	if err != nil {
		return false, err
	}
	return facts.AIOff, nil
}

// suggestVocabulary is the owner's non-archived categories and payees, payees
// by name so candidates of equal score come in a stable order.
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
		// the AI only ever sees the payee text: a row without one is not sent
		if aiPayee(c.link) == "" {
			continue
		}
		code, ok := codes[c.tx.AccountID]
		if !ok {
			code, err = s.accounts.AccountCurrencyCode(ctx, c.tx.AccountID)
			if err != nil {
				return nil, err
			}
			codes[c.tx.AccountID] = code
		}
		r := suggestRow{Link: c.link, Type: c.tx.Type, Currency: code, Account: names[c.tx.AccountID.String()]}
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
