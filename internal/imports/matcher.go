package imports

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MatcherConfig holds the four tunable thresholds (ECONUMO_IMPORT_*). They
// are guesses until real bank data has been through them, so they are
// configuration, not constants.
type MatcherConfig struct {
	MatchDays       int // ± window for the same-amount adopt
	TipDays         int // forward window (bank posts after the tap)
	TipTolerancePct int // percent of the tap amount
	TokenMinLength  int // shortest merchant token that must be contained
}

func DefaultMatcherConfig() MatcherConfig {
	return MatcherConfig{MatchDays: 3, TipDays: 5, TipTolerancePct: 20, TokenMinLength: 3}
}

type MatchKind int

const (
	MatchCreate MatchKind = iota
	MatchAdopt
	MatchTipAdopt
)

type MatchResult struct {
	Kind          MatchKind
	TransactionID vo.Id
	// CorrectAmount is set on a tip adopt when the transaction still carries
	// the tap-time amount, so the posted amount may overwrite it.
	CorrectAmount bool
}

// Match decides what an incoming event becomes, given the transactions the
// caller pre-selected on the same account. The exact-key check against the
// ledger happens before this (it needs the DB); Match is pure.
func Match(ev model.IngestEvent, sourceID vo.Id, candidates []model.ImportCandidate, cfg MatcherConfig) MatchResult {
	amount := vo.NewDecimal(ev.Amount)
	evDay := dayOf(ev.PostedAt)

	type scored struct {
		c    model.ImportCandidate
		days int
	}
	var adopt []scored
	for _, c := range candidates {
		if c.Type != ev.Type || linkedFrom(c, sourceID) {
			continue
		}
		if !vo.NewDecimal(c.Amount).Equals(amount) {
			continue
		}
		if d := absInt(daysBetween(dayOf(c.SpentAt), evDay)); d <= cfg.MatchDays {
			adopt = append(adopt, scored{c, d})
		}
	}
	if len(adopt) > 0 {
		sort.Slice(adopt, func(i, j int) bool {
			if adopt[i].days != adopt[j].days {
				return adopt[i].days < adopt[j].days
			}
			return adopt[i].c.TransactionID.String() < adopt[j].c.TransactionID.String()
		})
		return MatchResult{Kind: MatchAdopt, TransactionID: adopt[0].c.TransactionID}
	}

	type tipScored struct {
		c       model.ImportCandidate
		delta   vo.DecimalNumber
		days    int
		correct bool
	}
	var tips []tipScored
	evTokens := tokens(ev.Payee+" "+ev.Description, 1)
	pct := vo.NewDecimal(strconv.Itoa(cfg.TipTolerancePct))
	for _, c := range candidates {
		if c.Type != ev.Type || linkedFrom(c, sourceID) {
			continue
		}
		days := daysBetween(dayOf(c.SpentAt), evDay) // positive = bank posted after the tap
		if days < 0 || days > cfg.TipDays {
			continue
		}
		for _, l := range c.Links {
			if !model.ImportProviderIsPush(l.Provider) {
				continue
			}
			tap := vo.NewDecimal(l.ExternalAmount)
			delta := amount.Sub(tap).Abs()
			if delta.IsGreaterThan(tap.Mul(pct).Div(vo.NewDecimal("100"))) {
				continue
			}
			need := tokens(l.ExternalPayee, cfg.TokenMinLength)
			if len(need) == 0 || !containsAll(evTokens, need) {
				continue
			}
			tips = append(tips, tipScored{c: c, delta: delta, days: days, correct: vo.NewDecimal(c.Amount).Equals(tap)})
		}
	}
	if len(tips) == 0 {
		return MatchResult{Kind: MatchCreate}
	}
	sort.Slice(tips, func(i, j int) bool {
		if !tips[i].delta.Equals(tips[j].delta) {
			return tips[i].delta.IsLessThan(tips[j].delta)
		}
		if tips[i].days != tips[j].days {
			return tips[i].days < tips[j].days
		}
		return tips[i].c.TransactionID.String() < tips[j].c.TransactionID.String()
	})
	best := tips[0]
	return MatchResult{Kind: MatchTipAdopt, TransactionID: best.c.TransactionID, CorrectAmount: best.correct}
}

func linkedFrom(c model.ImportCandidate, sourceID vo.Id) bool {
	for _, l := range c.Links {
		if l.SourceID.Equal(sourceID) {
			return true
		}
	}
	return false
}

func dayOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// daysBetween returns b - a in whole days; both are midnight UTC.
func daysBetween(a, b time.Time) int { return int(b.Sub(a).Hours() / 24) }

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// tokens lowercases, turns every non-ASCII-alphanumeric rune into a space,
// splits, and keeps tokens of at least minLen runes. Deterministic; no regex.
// Non-Latin merchant names therefore yield no tokens and never tip-match —
// they fall through to create, which is the safe side.
func tokens(s string, minLen int) map[string]bool {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	out := map[string]bool{}
	for _, tok := range strings.Fields(b.String()) {
		if len(tok) >= minLen {
			out[tok] = true
		}
	}
	return out
}

func containsAll(have, need map[string]bool) bool {
	for tok := range need {
		if !have[tok] {
			return false
		}
	}
	return true
}
