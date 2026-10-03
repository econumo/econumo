package imports_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

var (
	srcBank  = vo.MustParseId("0c000000-0000-0000-0000-00000000000b")
	srcPhone = vo.MustParseId("0c000000-0000-0000-0000-00000000000f")
	txA      = vo.MustParseId("d0000000-0000-0000-0000-00000000000a")
	txB      = vo.MustParseId("d0000000-0000-0000-0000-00000000000b")
	day0     = time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)
)

func day(n int) time.Time { return day0.AddDate(0, 0, n) }

func event(amount string, posted time.Time, payee string) model.IngestEvent {
	return model.IngestEvent{ExternalAccountID: "card", ExternalTransactionID: "x", Type: model.TransactionTypeExpense,
		Amount: amount, Currency: "USD", PostedAt: posted, Payee: payee}
}

func pushLink(amount, payee string) model.ImportCandidateLink {
	return model.ImportCandidateLink{SourceID: srcPhone, Provider: model.ImportProviderAppleWallet, ExternalAmount: amount, ExternalPayee: payee}
}

func TestMatch_NoCandidatesCreates(t *testing.T) {
	got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, nil, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchCreate {
		t.Fatalf("got %+v, want create", got)
	}
}

func TestMatch_AdoptsSameAmountWithinWindow(t *testing.T) {
	cands := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(-3)},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(-1)},
	}
	got := imports.Match(event("4.50", day(0), "COFFEE"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchAdopt || !got.TransactionID.Equal(txB) || got.CorrectAmount {
		t.Fatalf("got %+v, want adopt txB (nearest date)", got)
	}
}

func TestMatch_AdoptRespectsWindowAndType(t *testing.T) {
	cfg := imports.DefaultMatcherConfig()
	outside := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(-4)}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, outside, cfg); got.Kind != imports.MatchCreate {
		t.Fatalf("4 days away with MatchDays=3: got %+v, want create", got)
	}
	income := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeIncome, Amount: "4.5", SpentAt: day(0)}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, income, cfg); got.Kind != imports.MatchCreate {
		t.Fatalf("income candidate for an expense event: got %+v, want create", got)
	}
	cfg.MatchDays = 0
	sameDay := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(0).Add(-14 * time.Hour)}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, sameDay, cfg); got.Kind != imports.MatchAdopt {
		t.Fatalf("MatchDays=0 must still match the same calendar day: got %+v", got)
	}
}

func TestMatch_SkipsCandidateAlreadyLinkedFromSameSource(t *testing.T) {
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(0),
		Links: []model.ImportCandidateLink{{SourceID: srcBank, Provider: model.ImportProviderSimpleFIN, ExternalAmount: "4.5"}}}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, cands, imports.DefaultMatcherConfig()); got.Kind != imports.MatchCreate {
		t.Fatalf("got %+v, want create", got)
	}
}

func TestMatch_TipAdopt(t *testing.T) {
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}}}
	got := imports.Match(event("46", day(2), "SQ *STARBUCKS 12345 SEATTLE WA"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchTipAdopt || !got.TransactionID.Equal(txA) || !got.CorrectAmount {
		t.Fatalf("got %+v, want tip-adopt txA with amount correction", got)
	}
}

func TestMatch_TipAdoptKeepsUserEditedAmount(t *testing.T) {
	// The user changed 40 -> 41 after the tap: link, but do not overwrite.
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "41", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}}}
	got := imports.Match(event("46", day(2), "STARBUCKS SEATTLE"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchTipAdopt || got.CorrectAmount {
		t.Fatalf("got %+v, want tip-adopt without correction", got)
	}
}

func TestMatch_TipAdoptBounds(t *testing.T) {
	cfg := imports.DefaultMatcherConfig()
	base := func() []model.ImportCandidate {
		return []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
			Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}}}
	}
	cases := []struct {
		name string
		ev   model.IngestEvent
		want imports.MatchKind
	}{
		{"posted before tap", event("46", day(-1), "STARBUCKS"), imports.MatchCreate},
		{"posted on tap day", event("46", day(0), "STARBUCKS"), imports.MatchTipAdopt},
		{"posted at +5 days", event("46", day(5), "STARBUCKS"), imports.MatchTipAdopt},
		{"posted at +6 days", event("46", day(6), "STARBUCKS"), imports.MatchCreate},
		{"amount +20% exactly", event("48", day(1), "STARBUCKS"), imports.MatchTipAdopt},
		{"amount +20.01%", event("48.004", day(1), "STARBUCKS"), imports.MatchCreate},
		{"amount -20% exactly", event("32", day(1), "STARBUCKS"), imports.MatchTipAdopt},
		{"merchant token missing", event("46", day(1), "SQ COFFEE SEATTLE"), imports.MatchCreate},
		{"merchant in description", model.IngestEvent{Type: model.TransactionTypeExpense, Amount: "46", PostedAt: day(1), Payee: "SQ", Description: "starbucks seattle"}, imports.MatchTipAdopt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := imports.Match(tc.ev, srcBank, base(), cfg); got.Kind != tc.want {
				t.Fatalf("got %+v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatch_TipAdoptTokenRules(t *testing.T) {
	cfg := imports.DefaultMatcherConfig()
	// "Sq" is shorter than TokenMinLength, so only "starbucks" must be present.
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "Sq Starbucks")}}}
	if got := imports.Match(event("46", day(1), "STARBUCKS #1"), srcBank, cands, cfg); got.Kind != imports.MatchTipAdopt {
		t.Fatalf("short tokens must be ignored: got %+v", got)
	}
	// A push payee with no qualifying token can never match: nothing to contain.
	empty := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "7-11")}}}
	if got := imports.Match(event("46", day(1), "7 11 STORE"), srcBank, empty, cfg); got.Kind != imports.MatchCreate {
		t.Fatalf("no qualifying push token: got %+v, want create", got)
	}
	cfg.TokenMinLength = 1
	if got := imports.Match(event("46", day(1), "7 11 STORE"), srcBank, empty, cfg); got.Kind != imports.MatchTipAdopt {
		t.Fatalf("TokenMinLength=1 must accept single-char tokens: got %+v", got)
	}
}

func TestMatch_TipAdoptPicksSmallestAmountDeltaThenNearestDate(t *testing.T) {
	cands := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "45", SpentAt: day(-3), Links: []model.ImportCandidateLink{pushLink("45", "Starbucks")}},
	}
	got := imports.Match(event("46", day(1), "STARBUCKS"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchTipAdopt || !got.TransactionID.Equal(txB) {
		t.Fatalf("got %+v, want txB (delta 1 beats delta 6)", got)
	}
	tie := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(-2), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
	}
	got = imports.Match(event("46", day(1), "STARBUCKS"), srcBank, tie, imports.DefaultMatcherConfig())
	if !got.TransactionID.Equal(txB) {
		t.Fatalf("got %+v, want txB (nearest date breaks the amount tie)", got)
	}
}

func TestMatch_ExactAmountBeatsTip(t *testing.T) {
	cands := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "46", SpentAt: day(1)},
	}
	got := imports.Match(event("46", day(1), "STARBUCKS"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchAdopt || !got.TransactionID.Equal(txB) {
		t.Fatalf("got %+v, want plain adopt of txB", got)
	}
}
