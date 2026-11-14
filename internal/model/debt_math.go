package model

import (
	"errors"
	"math"
	"sort"
	"time"
)

var ErrPaymentsDontCover = errors.New("payments do not cover the balance")

type MathSnapshot struct {
	AsOf             time.Time
	Balance, Payment float64
	Schedule         RecurringSchedule
	Last             time.Time
	// Digits are the debt currency's fraction digits: payments within half
	// its smallest unit of the balance are interest-free.
	Digits int
	// Origin marks the loan's first snapshot: its balance is the amount
	// borrowed BEFORE the start day's payments (see splitFromOrigin), and its
	// first period accrues interest prorated by days.
	Origin bool
	// rate, when rateSet, is the rate in force from this snapshot: a
	// statement after the origin keeps the loan's rate (withLoanRate).
	rate    float64
	rateSet bool
	// rates are the loan's rate changes, sorted by From (WithRates).
	rates []MathRate
	// basePay, when basePaySet, is the payment before the first rate change:
	// the origin's, never a statement's (WithRates).
	basePay    float64
	basePaySet bool
}

// MathRate is a rate change: from From on, the rate per schedule period and
// the regular payment.
type MathRate struct {
	From       time.Time
	PeriodRate float64
	Payment    float64
}

type MathPayment struct {
	ID     string
	Date   time.Time
	Amount float64
	// Interest is the stored interest part; nil = not split yet.
	Interest *float64
}

type ProjectionInput struct {
	Snaps    []MathSnapshot
	Pays     []MathPayment
	Interest map[string]float64
	Today    time.Time
	Months   []time.Time        // period keys (first-of-month), ascending
	Plans    map[string]float64 // period key "YYYY-MM-01" -> planned
	// PeriodStartDay is the day the budget's periods start on (PeriodWindow);
	// 0 or 1 is the calendar month.
	PeriodStartDay int
	// Digits are the debt currency's fraction digits: a balance is paid off
	// when it rounds to 0 in them.
	Digits int
}

type ProjectionMonth struct {
	ClosingBalance           float64
	PaymentsLeft             int
	ScheduledTotal, StillDue float64
}

type Projection struct {
	CurrentBalance float64
	PaymentsLeft   int
	// PeriodsPaid counts the due windows (latest terms' schedule) holding at
	// least one payment dated up to today.
	PeriodsPaid                               int
	MoneyLeft, OverpaymentLeft, OverpaidSoFar float64
	// ExtraPaid is what the payments dated up to today went above the regular
	// payment (extraPaid).
	ExtraPaid                  float64
	PayoffDate, BasePayoffDate time.Time
	// NextDueDate is the first projected payment; an unpaid due already past
	// stays itself (overdue).
	NextDueDate         time.Time
	BaseOverpaymentLeft float64
	Months              map[string]ProjectionMonth
	PaidOff             bool
	PayoffMonth         time.Time
	// PaidOffAt is the day the debt was paid off (PayoffMonth's day).
	PaidOffAt time.Time
	// Payments are the projected payments with the plans, soonest first: the
	// regular ones (a planned extra riding on its period's first) and, after
	// them in date order, an extra of a period with no regular payment.
	Payments []ProjectedPayment
}

// ProjectedPayment is one payment of the projection; Extra is the planned
// extra part of Payment and Balance what is owed after it.
type ProjectedPayment struct {
	Date                                         time.Time
	Payment, Interest, Principal, Extra, Balance float64
}

const (
	maxProjectMonths  = 1200
	monthKeyLayout    = "2006-01-02"
	windowSearchWidth = 370
)

// DebtTermTooLong reports a last payment date past the projection's horizon,
// which could never be projected to the end.
func DebtTermTooLong(asOf, last time.Time) bool {
	return debtDay(last).After(debtDay(asOf).AddDate(0, maxProjectMonths, 0))
}

func debtDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func scheduleMonths(s RecurringSchedule) int {
	switch s {
	case RecurringScheduleQuarterly:
		return 3
	case RecurringScheduleYearly:
		return 12
	}
	return 1
}

// shiftDue moves a due date one period (dir = +1/-1). Month-based schedules
// keep the anchor day, clamped to the target month's length.
func shiftDue(s RecurringSchedule, anchor int, d time.Time, dir int) time.Time {
	switch s {
	case RecurringScheduleWeekly:
		return d.AddDate(0, 0, 7*dir)
	case RecurringScheduleBiweekly:
		return d.AddDate(0, 0, 14*dir)
	}
	return anchoredMonth(d.Year(), d.Month()+time.Month(scheduleMonths(s)*dir), anchor)
}

func anchoredMonth(year int, month time.Month, anchor int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	if dim := daysInMonth(first.Year(), first.Month()); anchor > dim {
		anchor = dim
	}
	return time.Date(first.Year(), first.Month(), anchor, 0, 0, 0, 0, time.UTC)
}

// DueDatesBetween returns the due dates d with from < d <= to. The series is
// anchored on last (walked back from it, extended past it with NextOccurrence).
func DueDatesBetween(s RecurringSchedule, last time.Time, from, to time.Time) []time.Time {
	last, from, to = debtDay(last), debtDay(from), debtDay(to)
	anchor := last.Day()
	var out []time.Time
	for k := 0; ; k++ {
		var d time.Time
		switch s {
		case RecurringScheduleWeekly:
			d = last.AddDate(0, 0, -7*k)
		case RecurringScheduleBiweekly:
			d = last.AddDate(0, 0, -14*k)
		default:
			d = anchoredMonth(last.Year(), last.Month()-time.Month(scheduleMonths(s)*k), anchor)
		}
		if !d.After(from) {
			break
		}
		if !d.After(to) {
			out = append(out, d)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	for d := NextOccurrence(last, s, int16(anchor)); !d.After(to); d = NextOccurrence(d, s, int16(anchor)) {
		if d.After(from) {
			out = append(out, d)
		}
	}
	return out
}

// DueWindow is the half-open range [start, end) of dates that pay the given
// due: from halfway after the previous due to halfway before the next.
func DueWindow(s RecurringSchedule, last, due time.Time) (start, end time.Time) {
	due = debtDay(due)
	anchor := debtDay(last).Day()
	prev, next := shiftDue(s, anchor, due, -1), shiftDue(s, anchor, due, 1)
	start = prev.AddDate(0, 0, int(due.Sub(prev).Hours()/24)/2)
	end = due.AddDate(0, 0, int(next.Sub(due).Hours()/24)/2)
	return start, end
}

func dueContaining(s RecurringSchedule, last, t time.Time) time.Time {
	t = debtDay(t)
	dues := DueDatesBetween(s, last, t.AddDate(0, 0, -windowSearchWidth), t.AddDate(0, 0, windowSearchWidth))
	for _, d := range dues {
		if st, en := DueWindow(s, last, d); !t.Before(st) && t.Before(en) {
			return d
		}
	}
	return time.Time{}
}

// ImpliedRate solves the periodic rate at which n payments repay balance;
// digits are the currency's fraction digits, so a difference below half its
// smallest unit is rounding, not interest or a shortfall.
func ImpliedRate(balance, payment float64, n, digits int) (float64, error) {
	return impliedRate(balance, payment, n, digits, 1)
}

// impliedRate solves the rate when the first period accrues only the share
// first of a full period's interest.
func impliedRate(balance, payment float64, n, digits int, first float64) (float64, error) {
	if balance <= 0 {
		return 0, nil
	}
	total := payment * float64(n)
	if math.Abs(total-balance) < halfUnit(digits) {
		return 0, nil
	}
	if total < balance {
		return 0, ErrPaymentsDontCover
	}
	// B(1 + first*r) = P + P*annuity(n-1, r)
	pv := func(r float64) float64 {
		rest := payment * (-math.Expm1(-float64(n-1) * math.Log1p(r))) / r
		return (payment + rest) / (1 + first*r)
	}
	lo, hi := 0.0, 1.0
	// pv falls as r grows: widen until the bracket holds the solution.
	for hi < 1e9 && pv(hi) > balance {
		lo, hi = hi, hi*2
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if pv(mid) > balance {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, nil
}

// originFirst is the first due after an origin and the share of a full
// period (the one ending on that due) its interest is prorated to: the days
// from the start to that due over the period's days.
func originFirst(s MathSnapshot) (time.Time, float64) {
	start := debtDay(s.AsOf)
	dues := DueDatesBetween(s.Schedule, s.Last, start, start.AddDate(0, 0, windowSearchWidth))
	if len(dues) == 0 {
		return time.Time{}, 1
	}
	first := dues[0]
	prev := shiftDue(s.Schedule, debtDay(s.Last).Day(), first, -1)
	full := first.Sub(prev).Hours() / 24
	if full <= 0 {
		return first, 1
	}
	return first, math.Min(1, first.Sub(start).Hours()/24/full)
}

// SnapshotRate is a snapshot's implied rate per schedule period; an origin
// solves its prorated first period. ErrPaymentsDontCover when the dues after
// the as-of day cannot repay the balance.
func SnapshotRate(s MathSnapshot) (float64, error) {
	n := len(DueDatesBetween(s.Schedule, s.Last, s.AsOf, s.Last))
	first := 1.0
	if s.Origin {
		_, first = originFirst(s)
	}
	return impliedRate(s.Balance, s.Payment, n, s.Digits, first)
}

// snapshotRate is the rate in force from s: the loan's when withLoanRate set
// it, else s's own implied rate.
func snapshotRate(s MathSnapshot) float64 {
	if s.rateSet {
		return s.rate
	}
	r, err := SnapshotRate(s)
	if err != nil {
		return 0
	}
	return r
}

// PeriodRate is the rate per schedule period in force after the latest
// snapshot and the latest rate change.
func PeriodRate(snaps []MathSnapshot) float64 {
	snaps = sortedSnaps(snaps)
	if len(snaps) == 0 {
		return 0
	}
	latest := snaps[len(snaps)-1]
	day := latest.AsOf
	if n := len(latest.rates); n > 0 && latest.rates[n-1].From.After(day) {
		day = latest.rates[n-1].From
	}
	return latest.rateOn(day)
}

// WithRates attaches the loan's rate changes to every snapshot. From then on
// the payment before the first change is the origin's: a statement's payment
// is never read, so one stored under a change that is later deleted cannot
// keep that change's payment alive. The rule holds only through WithRates, so
// a debt's snapshots must pass through it even with no rate changes.
func WithRates(snaps []MathSnapshot, rates []MathRate) []MathSnapshot {
	sorted := make([]MathRate, len(rates))
	for i, r := range rates {
		r.From = debtDay(r.From)
		sorted[i] = r
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].From.Before(sorted[j].From) })
	out := append([]MathSnapshot(nil), snaps...)
	originPay, hasOrigin := 0.0, false
	for _, s := range snaps {
		if s.Origin {
			originPay, hasOrigin = s.Payment, true
			break
		}
	}
	for i := range out {
		out[i].rates = sorted
		out[i].basePay, out[i].basePaySet = originPay, hasOrigin
	}
	return out
}

func periodsPerYear(s RecurringSchedule) float64 {
	switch s {
	case RecurringScheduleWeekly:
		return 52
	case RecurringScheduleBiweekly:
		return 26
	case RecurringScheduleQuarterly:
		return 4
	case RecurringScheduleYearly:
		return 1
	}
	return 12
}

func AnnualToPeriodRate(annualPercent float64, s RecurringSchedule) float64 {
	return annualPercent / 100 / periodsPerYear(s)
}

func PeriodToAnnualRate(periodRate float64, s RecurringSchedule) float64 {
	return periodRate * periodsPerYear(s) * 100
}

func PeriodRateOn(snaps []MathSnapshot, day time.Time) float64 {
	snaps, day = sortedSnaps(snaps), debtDay(day)
	if len(snaps) == 0 {
		return 0
	}
	return snapshotOn(snaps, day).rateOn(day)
}

func PaymentOn(snaps []MathSnapshot, day time.Time) float64 {
	snaps, day = sortedSnaps(snaps), debtDay(day)
	if len(snaps) == 0 {
		return 0
	}
	return snapshotOn(snaps, day).paymentOn(day)
}

func DueWindowContaining(s RecurringSchedule, last, day time.Time) (start, end time.Time) {
	due := dueContaining(s, last, day)
	if due.IsZero() {
		return time.Time{}, time.Time{}
	}
	return DueWindow(s, last, due)
}

// snapshotOn is the snapshot in force on day among sorted snaps; the first
// one before them all.
func snapshotOn(snaps []MathSnapshot, day time.Time) MathSnapshot {
	terms := snaps[0]
	for _, s := range snaps {
		if !s.AsOf.After(day) {
			terms = s
		}
	}
	return terms
}

// rateOn is the rate in force on day: the latest change on or before it, else
// the snapshot's (the loan's) rate.
func (s MathSnapshot) rateOn(day time.Time) float64 {
	r := snapshotRate(s)
	for _, c := range s.rates {
		if c.From.After(day) {
			break
		}
		r = c.PeriodRate
	}
	return r
}

// paymentOn is the regular payment in force on day: the latest change's on or
// before it, else the origin's under WithRates, else the snapshot's own.
func (s MathSnapshot) paymentOn(day time.Time) float64 {
	p := s.Payment
	if s.basePaySet {
		p = s.basePay
	}
	for _, c := range s.rates {
		if c.From.After(day) {
			break
		}
		p = c.Payment
	}
	return p
}

func (s MathSnapshot) changesAfter(day time.Time) bool {
	n := len(s.rates)
	return n > 0 && s.rates[n-1].From.After(day)
}

func debtDays(from, to time.Time) float64 { return to.Sub(from).Hours() / 24 }

// windowInterest is the interest on balance of the window ending at due: each
// day of [from, due) accrues the rate in force on it, over the days of the
// full period [prevDue, due). A change inside the period is so prorated by
// days, and one dated on a due starts with the period after it.
func windowInterest(s MathSnapshot, balance float64, from, prevDue, due time.Time) float64 {
	if from.Before(prevDue) {
		from = prevDue
	}
	full := debtDays(prevDue, due)
	r := s.rateOn(from)
	if full <= 0 {
		return balance * r
	}
	var sum float64
	cur := from
	for _, c := range s.rates {
		if !c.From.After(from) {
			continue
		}
		if !c.From.Before(due) {
			break
		}
		sum += r * debtDays(cur, c.From)
		cur, r = c.From, c.PeriodRate
	}
	// One rate over the window: the same arithmetic as before rate changes
	// existed, so the figures stay identical to the cent.
	if cur.Equal(from) {
		if from.Equal(prevDue) {
			return balance * r
		}
		return balance * r * (debtDays(from, due) / full)
	}
	sum += r * debtDays(cur, due)
	return balance * sum / full
}

// dueInterest is windowInterest for the window ending at due; first is the
// origin's first due (originFirst), whose window accrues from the start.
func dueInterest(s MathSnapshot, balance float64, due, first time.Time) float64 {
	prev := shiftDue(s.Schedule, debtDay(s.Last).Day(), due, -1)
	from := prev
	if s.Origin && due.Equal(first) {
		from = debtDay(s.AsOf)
	}
	return windowInterest(s, balance, from, prev, due)
}

// withLoanRate gives every snapshot the origin's implied rate,
// the loan's: a statement corrects what is owed, not the contract, so a
// balance below Econumo's shortens the loan rather than raising its rate.
// Without an origin each snapshot keeps its own implied rate.
func withLoanRate(snaps []MathSnapshot) {
	for _, o := range snaps {
		if !o.Origin {
			continue
		}
		r := snapshotRate(o)
		for i := range snaps {
			snaps[i].rate, snaps[i].rateSet = r, true
		}
		return
	}
}

// StatementRepayable reports ErrPaymentsDontCover when s's regular payment can
// never repay its balance at the rate in force (the loan's, origin's, before
// any change): it does not exceed a period's interest with no change to come,
// or the balance outlasts the projection's horizon. Each period accrues in
// full (a statement balance is the principal still owed).
func StatementRepayable(origin, s MathSnapshot) error {
	r, err := SnapshotRate(origin)
	if err != nil {
		return err
	}
	s.rate, s.rateSet = r, true
	eps := settledBelow(s.Digits)
	balance := s.Balance
	for _, due := range DueDatesBetween(s.Schedule, s.Last, s.AsOf, debtDay(s.AsOf).AddDate(0, maxProjectMonths, 0)) {
		if balance < eps {
			return nil
		}
		interest := dueInterest(s, balance, due, time.Time{})
		payment := s.paymentOn(due)
		if payment <= interest && !s.changesAfter(due) {
			return ErrPaymentsDontCover
		}
		balance += interest - payment
	}
	if balance < eps {
		return nil
	}
	return ErrPaymentsDontCover
}

func sortedSnaps(snaps []MathSnapshot) []MathSnapshot {
	out := append([]MathSnapshot(nil), snaps...)
	for i := range out {
		out[i].AsOf = debtDay(out[i].AsOf)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AsOf.Before(out[j].AsOf) })
	withLoanRate(out)
	for i := range out {
		if !out[i].rateSet {
			out[i].rate, out[i].rateSet = snapshotRate(out[i]), true
		}
	}
	return out
}

func sortedPays(pays []MathPayment) []MathPayment {
	out := append([]MathPayment(nil), pays...)
	for i := range out {
		out[i].Date = debtDay(out[i].Date)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

type payGroup struct {
	due  time.Time
	pays []MathPayment
}

// groupByWindow splits date-ordered payments into runs sharing one due window.
func groupByWindow(s MathSnapshot, pays []MathPayment) []payGroup {
	var groups []payGroup
	for _, p := range pays {
		due := dueContaining(s.Schedule, s.Last, p.Date)
		if n := len(groups); n > 0 && groups[n-1].due.Equal(due) {
			groups[n-1].pays = append(groups[n-1].pays, p)
			continue
		}
		groups = append(groups, payGroup{due: due, pays: []MathPayment{p}})
	}
	return groups
}

// allocate hands the window's interest to its payments: a stored interest is
// kept (capped at the amount) and reserved first, so a payment split later
// takes only what is left of the window's interest, whatever its date.
func allocate(pays []MathPayment, due float64, out map[string]float64) float64 {
	for _, p := range pays {
		if p.Interest != nil {
			out[p.ID] = math.Min(math.Max(*p.Interest, 0), p.Amount)
			due -= out[p.ID]
		}
	}
	for _, p := range pays {
		if p.Interest == nil {
			out[p.ID] = math.Min(math.Max(due, 0), p.Amount)
			due -= out[p.ID]
		}
	}
	return due
}

// onOrAfterSnapshot: a payment dated on an origin's day comes after it (the
// origin is the amount borrowed before that day's payments); on any other
// snapshot's day it is already in the balance.
func onOrAfterSnapshot(s MathSnapshot, d time.Time) bool {
	if s.Origin {
		return !d.Before(s.AsOf)
	}
	return d.After(s.AsOf)
}

// SplitPayments returns the interest part of every payment.
func SplitPayments(snaps []MathSnapshot, pays []MathPayment) map[string]float64 {
	out := make(map[string]float64, len(pays))
	snaps, pays = sortedSnaps(snaps), sortedPays(pays)
	if len(snaps) == 0 {
		return out
	}

	var backward []MathPayment
	for _, p := range pays {
		if !onOrAfterSnapshot(snaps[0], p.Date) {
			backward = append(backward, p)
		}
	}
	splitBackward(snaps[0], backward, out)

	for k, s := range snaps {
		var seg []MathPayment
		for _, p := range pays {
			if onOrAfterSnapshot(s, p.Date) && (k == len(snaps)-1 || !onOrAfterSnapshot(snaps[k+1], p.Date)) {
				seg = append(seg, p)
			}
		}
		splitForward(s, seg, pays, out)
	}
	return out
}

func splitForward(s MathSnapshot, seg, all []MathPayment, out map[string]float64) {
	if len(seg) == 0 {
		return
	}
	if s.Origin {
		splitFromOrigin(s, seg, out)
		return
	}
	balance := s.Balance
	accrued := dueContaining(s.Schedule, s.Last, s.AsOf)
	accruedBefore := false
	for _, p := range all {
		if !p.Date.After(s.AsOf) && dueContaining(s.Schedule, s.Last, p.Date).Equal(accrued) {
			accruedBefore = true
			break
		}
	}
	for _, g := range groupByWindow(s, seg) {
		due := dueInterest(s, balance, g.due, time.Time{})
		if accruedBefore && g.due.Equal(accrued) {
			due = 0
		}
		allocate(g.pays, due, out)
		for _, p := range g.pays {
			balance -= p.Amount - out[p.ID]
		}
	}
}

// splitFromOrigin: the start day's payments pay the interest owed for the time
// before the start first (a mortgage's interest adjustment), at most a full
// period's on the amount borrowed, and only the rest comes off the balance; a
// window whose due is on or before the start accrues nothing (the loan had not
// begun); the first due's window accrues its prorated share; later ones a full
// period's interest.
func splitFromOrigin(s MathSnapshot, seg []MathPayment, out map[string]float64) {
	start := debtDay(s.AsOf)
	first, _ := originFirst(s)
	balance := s.Balance
	var startDay, rest []MathPayment
	for _, p := range seg {
		if p.Date.Equal(start) {
			startDay = append(startDay, p)
		} else {
			rest = append(rest, p)
		}
	}
	allocate(startDay, balance*s.rateOn(start), out)
	for _, p := range startDay {
		balance -= p.Amount - out[p.ID]
	}
	for _, g := range groupByWindow(s, rest) {
		due := 0.0
		if g.due.After(start) {
			due = dueInterest(s, balance, g.due, first)
		}
		allocate(g.pays, due, out)
		for _, p := range g.pays {
			balance -= p.Amount - out[p.ID]
		}
	}
}

func splitBackward(s MathSnapshot, seg []MathPayment, out map[string]float64) {
	if len(seg) == 0 {
		return
	}
	groups := groupByWindow(s, seg)
	end := s.Balance
	for i := len(groups) - 1; i >= 0; i-- {
		r := dueInterest(s, 1, groups[i].due, time.Time{})
		var total float64
		for _, p := range groups[i].pays {
			total += p.Amount
		}
		due := (end + total) / (1 + r) * r
		if total < due {
			due = total
		}
		allocate(groups[i].pays, due, out)
		// A stored interest may differ from the fresh one: the earlier window
		// ends where the debt parts actually allocated leave the balance.
		for _, p := range groups[i].pays {
			end += p.Amount - out[p.ID]
		}
	}
}

func BalanceAt(snaps []MathSnapshot, pays []MathPayment, interest map[string]float64, t time.Time) float64 {
	snaps, pays, t = sortedSnaps(snaps), sortedPays(pays), debtDay(t)
	if len(snaps) == 0 {
		return 0
	}
	idx := -1
	for i, s := range snaps {
		if !s.AsOf.After(t) {
			idx = i
		}
	}
	if idx < 0 {
		balance := snaps[0].Balance
		for _, p := range pays {
			if p.Date.After(t) && !onOrAfterSnapshot(snaps[0], p.Date) {
				balance += p.Amount - interest[p.ID]
			}
		}
		return balance
	}
	balance := snaps[idx].Balance
	for _, p := range pays {
		if onOrAfterSnapshot(snaps[idx], p.Date) && !p.Date.After(t) {
			balance -= p.Amount - interest[p.ID]
		}
	}
	return balance
}

func roundDigits(x float64, digits int) float64 {
	p := math.Pow10(digits)
	return math.Round(x*p) / p
}

// halfUnit is half the currency's smallest unit: a balance at or below it
// rounds to 0.
func halfUnit(digits int) float64 { return math.Pow10(-digits) / 2 }

// settledBelow is the balance under which a debt is settled: one that rounds
// to at most ONE smallest unit. Interest stored rounded to the unit against
// the float projection can leave such a residue after the last due; the
// last payment absorbs it rather than a due of one unit being projected.
func settledBelow(digits int) float64 { return 3 * halfUnit(digits) }

func monthKey(m time.Time) string { return m.Format(monthKeyLayout) }

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

type projStep struct {
	date time.Time
	pay  float64
}

type projRun struct {
	steps      []projStep
	payments   []ProjectedPayment
	extraTotal float64
	closing    map[string]float64
	endBalance float64
	payoffDate time.Time
}

// moneyLeft is what the projected payments pay; a run cut off by the horizon
// adds the balance it left, which is still owed (a lower bound then).
func (r projRun) moneyLeft() float64 {
	total := r.extraTotal + r.endBalance
	for _, s := range r.steps {
		total += s.pay
	}
	return total
}

// runProjection amortises balance forward from firstDue, period by period
// (budget periods starting on startDay). An extra goes fully to principal right
// after the period's first projected payment (the regular payment already took
// that schedule period's interest), or at period end when the period has no
// projected payment.
func runProjection(s MathSnapshot, balance, eps float64, today, firstDue time.Time, extras map[string]float64, startDay int) projRun {
	run := projRun{closing: map[string]float64{}, endBalance: balance}
	if balance < eps {
		run.endBalance = 0
		return run
	}
	anchor := int16(debtDay(s.Last).Day())
	due := firstDue
	// projected from the origin, the first due accrues its prorated share
	var first time.Time
	if s.Origin {
		first, _ = originFirst(s)
	}
	month := PeriodOf(today, startDay)
	for i := 0; i < maxProjectMonths && balance >= eps; i++ {
		_, next := PeriodWindow(month, startDay)
		extra := extras[monthKey(month)]
		stepped := false
		for balance >= eps && due.Before(next) {
			interest := dueInterest(s, balance, due, first)
			pay := math.Min(s.paymentOn(due), balance+interest)
			balance = balance + interest - pay
			if balance < eps {
				// the residue of at most one unit rides on this payment
				pay += balance
				balance = 0
			}
			run.steps = append(run.steps, projStep{date: due, pay: pay})
			if balance <= 0 {
				balance, run.payoffDate = 0, laterOf(due, today)
				run.payments = append(run.payments, ProjectedPayment{Date: due, Payment: pay, Interest: interest, Principal: pay - interest})
				break
			}
			payment := ProjectedPayment{Date: due, Payment: pay, Interest: interest, Principal: pay - interest, Balance: balance}
			if !stepped && extra > 0 {
				applied := math.Min(extra, balance)
				balance -= applied
				run.extraTotal += applied
				payment.Payment += applied
				payment.Principal += applied
				payment.Extra = applied
				payment.Balance = balance
				if balance < eps {
					run.extraTotal += balance
					payment.Payment += balance
					payment.Principal += balance
					payment.Extra += balance
					balance, run.payoffDate = 0, laterOf(due, today)
					payment.Balance = 0
					run.payments = append(run.payments, payment)
					break
				}
			}
			run.payments = append(run.payments, payment)
			stepped = true
			due = NextOccurrence(due, s.Schedule, anchor)
		}
		if !stepped && balance >= eps && extra > 0 {
			applied := math.Min(extra, balance)
			balance -= applied
			if balance < eps {
				applied += balance
				balance = 0
				run.payoffDate = next.AddDate(0, 0, -1)
			}
			run.extraTotal += applied
			run.payments = append(run.payments, ProjectedPayment{Date: next.AddDate(0, 0, -1), Payment: applied, Principal: applied, Extra: applied, Balance: balance})
		}
		run.closing[monthKey(month)] = balance
		month = month.AddDate(0, 1, 0)
	}
	run.endBalance = balance
	return run
}

// laterOf keeps a projected payoff from landing before today: an overdue due
// that clears the balance has still to be paid.
func laterOf(due, today time.Time) time.Time {
	if due.Before(today) {
		return today
	}
	return due
}

// payoffEvent finds the event (a payment or a snapshot) that took the balance
// to zero, provided nothing was paid after it. A zero snapshot settles the
// debt as of its day, whatever was paid before it.
func payoffEvent(snaps []MathSnapshot, pays []MathPayment, interest map[string]float64, today time.Time, digits int) (time.Time, bool) {
	seen := map[time.Time]bool{}
	zeroSnap := map[time.Time]bool{}
	var times []time.Time
	for _, s := range snaps {
		if s.AsOf.After(today) {
			continue
		}
		zeroSnap[s.AsOf] = roundDigits(s.Balance, digits) <= 0
		if !seen[s.AsOf] {
			seen[s.AsOf] = true
			times = append(times, s.AsOf)
		}
	}
	for _, p := range pays {
		if !p.Date.After(today) && !seen[p.Date] {
			seen[p.Date] = true
			times = append(times, p.Date)
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	var at time.Time
	found := false
	for i := len(times) - 1; i >= 0; i-- {
		if BalanceAt(snaps, pays, interest, times[i]) >= settledBelow(digits) {
			break
		}
		at, found = times[i], true
		if zeroSnap[times[i]] {
			break
		}
	}
	if !found {
		return time.Time{}, false
	}
	for _, p := range pays {
		if p.Date.After(at) && !p.Date.After(today) {
			return time.Time{}, false
		}
	}
	return at, true
}

func ProjectDebt(in ProjectionInput) Projection {
	out := Projection{Months: map[string]ProjectionMonth{}}
	snaps, pays := sortedSnaps(in.Snaps), sortedPays(in.Pays)
	if len(snaps) == 0 {
		return out
	}
	today := debtDay(in.Today)
	latest := snaps[len(snaps)-1]
	current := BalanceAt(snaps, pays, in.Interest, today)

	eps := settledBelow(in.Digits)
	if at, ok := payoffEvent(snaps, pays, in.Interest, today, in.Digits); ok && current < eps {
		out.PaidOff, out.PayoffMonth, out.PaidOffAt = true, monthStart(at), at
	}
	if current < 0 {
		current = 0
	}
	out.CurrentBalance = current
	paidDues := map[time.Time]bool{}
	for _, p := range pays {
		if !p.Date.After(today) {
			out.OverpaidSoFar += in.Interest[p.ID]
			if !instalment(snaps[0], p) {
				continue
			}
			if due := dueContaining(latest.Schedule, latest.Last, p.Date); !due.IsZero() {
				paidDues[due] = true
			}
		}
	}
	// A statement after the origin settles every due up to its date: the
	// payments behind it need not be recorded.
	if origin := snaps[0]; origin.Origin && len(snaps) > 1 && !latest.AsOf.After(today) {
		for _, due := range DueDatesBetween(latest.Schedule, latest.Last, origin.AsOf, latest.AsOf) {
			paidDues[due] = true
		}
	}
	out.PeriodsPaid = len(paidDues)
	out.ExtraPaid = extraPaid(snaps, pays, in.Interest, today)

	firstDue := firstUnpaidDue(latest, snaps[0], pays, today)
	sd := in.PeriodStartDay
	base := runProjection(latest, current, eps, today, firstDue, nil, sd)
	currentMonth := PeriodOf(today, sd)
	currentStart, currentEnd := PeriodWindow(currentMonth, sd)
	extras := map[string]float64{}
	for key, planned := range in.Plans {
		m, err := time.Parse(monthKeyLayout, key)
		if err != nil || m.Before(currentMonth) {
			continue
		}
		ws, we := PeriodWindow(m, sd)
		var covered float64
		for _, d := range periodDues(latest, snaps[0], ws, we) {
			covered += latest.paymentOn(d)
		}
		// This period's plan is also met by what was paid and is still due in
		// it; a due paid early last period keeps counting as scheduled here.
		if m.Equal(currentMonth) {
			covered = math.Max(covered, paidBetween(pays, currentStart, today)+stillDueBefore(base, currentEnd))
		}
		if extra := planned - covered; extra > 0 {
			extras[key] = extra
		}
	}
	run := runProjection(latest, current, eps, today, firstDue, extras, sd)

	out.PaymentsLeft = len(run.steps)
	out.Payments = run.payments
	out.MoneyLeft = run.moneyLeft()
	out.OverpaymentLeft = out.MoneyLeft - current
	out.PayoffDate = run.payoffDate
	if len(run.steps) > 0 {
		out.NextDueDate = run.steps[0].date
	}
	out.BasePayoffDate = base.payoffDate
	out.BaseOverpaymentLeft = base.moneyLeft() - current

	stepPay := make(map[time.Time]float64, len(run.steps))
	for _, st := range run.steps {
		stepPay[st.date] = st.pay
	}
	var lastStep time.Time
	if n := len(run.steps); n > 0 && run.endBalance <= 0 {
		lastStep = run.steps[n-1].date
	}
	for _, m := range in.Months {
		m = monthStart(m)
		ws, we := PeriodWindow(m, sd)
		monthEnd := we.AddDate(0, 0, -1)
		pm := ProjectionMonth{}
		dues := periodDues(latest, snaps[0], ws, we)
		for _, d := range dues {
			pm.ScheduledTotal += latest.paymentOn(d)
		}
		// From the current period on, a due the projection pays counts its
		// projected payment (the last one can be smaller) and a due after the
		// payoff counts nothing; a due already paid keeps the regular payment.
		if !m.Before(currentMonth) {
			pm.ScheduledTotal = 0
			for _, d := range dues {
				if pay, ok := stepPay[d]; ok {
					pm.ScheduledTotal += pay
				} else if lastStep.IsZero() || !d.After(lastStep) {
					pm.ScheduledTotal += latest.paymentOn(d)
				}
			}
		}
		stepsReach := false
		for _, st := range run.steps {
			// An unpaid due before this period still has to be paid, so it
			// counts in the current period.
			date := st.date
			if date.Before(currentStart) {
				date = currentStart
			}
			if !date.Before(ws) {
				stepsReach = true
			}
			if !date.Before(ws) && !date.After(monthEnd) {
				pm.StillDue += st.pay
			}
		}
		// From the current period on, nothing is scheduled once the projection
		// has paid the debt off, whatever the term says.
		if !m.Before(currentMonth) && !stepsReach {
			pm.ScheduledTotal = 0
		}
		switch {
		case m.Before(currentMonth):
			pm.ClosingBalance = math.Max(BalanceAt(snaps, pays, in.Interest, monthEnd), 0)
			pm.PaymentsLeft = paymentsLeftAt(snaps, pm.ClosingBalance, monthEnd, eps)
		default:
			closing, ok := run.closing[monthKey(m)]
			if !ok {
				closing = run.endBalance
			}
			pm.ClosingBalance = closing
			// What is left after this period at the regular payment: a later
			// period's planned extra lowers the count in that period, not before.
			pm.PaymentsLeft = paymentsLeftAt(snaps, math.Max(closing, 0), monthEnd, eps)
		}
		out.Months[monthKey(m)] = pm
	}
	return out
}

// extraPaid sums, over the due windows (latest terms' schedule, as
// PeriodsPaid) holding payments dated up to today, what each window's
// payments went above the regular payment of the snapshot in force on its
// due; a window paid short adds nothing. It is measured by window, so a missed
// window followed by a double payment counts the second payment as extra: the
// money went above that window's payment, whatever was owed from before. A
// payment on the origin's day pays no due and counts what it took off the
// amount borrowed (its interest is the time before the start); the dues a
// statement settles count only the payments recorded in their windows.
func extraPaid(snaps []MathSnapshot, pays []MathPayment, interest map[string]float64, today time.Time) float64 {
	latest := snaps[len(snaps)-1]
	var total float64
	paid := map[time.Time]float64{}
	var dues []time.Time
	for _, p := range pays {
		if p.Date.After(today) {
			continue
		}
		if !instalment(snaps[0], p) {
			if principal := roundDigits(p.Amount-interest[p.ID], latest.Digits); principal > 0 {
				total += principal
			}
			continue
		}
		due := dueContaining(latest.Schedule, latest.Last, p.Date)
		if _, ok := paid[due]; !ok {
			dues = append(dues, due)
		}
		paid[due] += p.Amount
	}
	for _, due := range dues {
		terms := snapshotOn(snaps, due)
		if excess := roundDigits(paid[due]-terms.paymentOn(due), terms.Digits); excess > 0 {
			total += excess
		}
	}
	return total
}

// paymentsLeftAt counts the regular payments that clear balance under the
// terms in force on day t (the first snapshot's before it), up to the
// projection's horizon in time: a weekly loan has far more payments than
// months.
func paymentsLeftAt(snaps []MathSnapshot, balance float64, t time.Time, eps float64) int {
	terms := snapshotOn(snaps, t)
	if terms.paymentOn(t) <= 0 && !terms.changesAfter(t) {
		return 0
	}
	dues := DueDatesBetween(terms.Schedule, terms.Last, t, t.AddDate(0, maxProjectMonths, 0))
	horizon := len(dues)
	// before the origin's first due, that due accrues only its prorated share,
	// as SnapshotRate and the projection count it
	var first time.Time
	if terms.Origin && horizon > 0 {
		first, _ = originFirst(terms)
	}
	n := 0
	for ; balance >= eps && n < horizon; n++ {
		interest := dueInterest(terms, balance, dues[n], first)
		payment := terms.paymentOn(dues[n])
		if payment <= interest && !terms.changesAfter(dues[n]) {
			return horizon
		}
		balance += interest - math.Min(payment, balance+interest)
	}
	return n
}

// periodDues are the dues (latest terms) of the period [ws, we). A loan's
// dues come after its origin: one on the start day pays no due and one before
// it is not the loan's, so a period holding the origin schedules only the
// dues after it, and a period before it none.
func periodDues(latest, first MathSnapshot, ws, we time.Time) []time.Time {
	from := ws.AddDate(0, 0, -1)
	if start := debtDay(first.AsOf); first.Origin && from.Before(start) {
		from = start
	}
	return DueDatesBetween(latest.Schedule, latest.Last, from, we.AddDate(0, 0, -1))
}

// paidBetween sums the payments dated in [from, to].
func paidBetween(pays []MathPayment, from, to time.Time) float64 {
	var total float64
	for _, p := range pays {
		if !p.Date.Before(from) && !p.Date.After(to) {
			total += p.Amount
		}
	}
	return total
}

// stillDueBefore sums the projected regular payments dated before end,
// overdue ones included.
func stillDueBefore(run projRun, end time.Time) float64 {
	var total float64
	for _, st := range run.steps {
		if st.date.Before(end) {
			total += st.pay
		}
	}
	return total
}

// firstUnpaidDue is the earliest due, from the one whose window holds today,
// that falls after the snapshot and has no payment dated up to today in its
// window.
// first is the debt's first snapshot: a payment on the origin's day pays no
// due, whichever snapshot is the latest.
func firstUnpaidDue(s, first MathSnapshot, pays []MathPayment, today time.Time) time.Time {
	anchor := int16(debtDay(s.Last).Day())
	due := dueContaining(s.Schedule, s.Last, today)
	if due.IsZero() {
		due = today
	}
	for i := 0; i < maxProjectMonths*5; i++ {
		if due.After(s.AsOf) && !paidWindow(s, first, pays, due, today) {
			break
		}
		due = NextOccurrence(due, s.Schedule, anchor)
	}
	return due
}

// instalment: a payment on (or before) the origin's day is principal paid at
// the start, not a scheduled payment, so it pays no due.
func instalment(first MathSnapshot, p MathPayment) bool {
	return !first.Origin || p.Date.After(debtDay(first.AsOf))
}

func paidWindow(s, first MathSnapshot, pays []MathPayment, due, today time.Time) bool {
	start, end := DueWindow(s.Schedule, s.Last, due)
	for _, p := range pays {
		if instalment(first, p) && !p.Date.After(today) && !p.Date.Before(start) && p.Date.Before(end) {
			return true
		}
	}
	return false
}
