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
	Months   []time.Time        // first-of-month, ascending
	Plans    map[string]float64 // "YYYY-MM-01" -> planned
}

type ProjectionMonth struct {
	ClosingBalance           float64
	PaymentsLeft             int
	ScheduledTotal, StillDue float64
}

type Projection struct {
	CurrentBalance                            float64
	PaymentsLeft                              int
	MoneyLeft, OverpaymentLeft, OverpaidSoFar float64
	PayoffDate, BasePayoffDate                time.Time
	// NextDueDate is the first projected payment; an unpaid due already past
	// stays itself (overdue).
	NextDueDate         time.Time
	BaseOverpaymentLeft float64
	Months              map[string]ProjectionMonth
	PaidOff             bool
	PayoffMonth         time.Time
}

const (
	debtEpsilon       = 0.005
	maxProjectMonths  = 1200
	monthKeyLayout    = "2006-01-02"
	windowSearchWidth = 370
)

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

func ImpliedRate(balance, payment float64, n int) (float64, error) {
	if balance <= 0 {
		return 0, nil
	}
	total := payment * float64(n)
	if math.Abs(total-balance) < debtEpsilon {
		return 0, nil
	}
	if total < balance {
		return 0, ErrPaymentsDontCover
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		pv := payment * (-math.Expm1(-float64(n) * math.Log1p(mid))) / mid
		if pv > balance {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, nil
}

func snapshotRate(s MathSnapshot) float64 {
	n := len(DueDatesBetween(s.Schedule, s.Last, s.AsOf, s.Last))
	r, err := ImpliedRate(s.Balance, s.Payment, n)
	if err != nil {
		return 0
	}
	return r
}

// PeriodRate is the implied rate per schedule period of a snapshot.
func PeriodRate(s MathSnapshot) float64 { return snapshotRate(s) }

func sortedSnaps(snaps []MathSnapshot) []MathSnapshot {
	out := append([]MathSnapshot(nil), snaps...)
	for i := range out {
		out[i].AsOf = debtDay(out[i].AsOf)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AsOf.Before(out[j].AsOf) })
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

// allocate hands the window's interest to its payments in date order; stored
// interest wins and is capped at the amount.
func allocate(pays []MathPayment, due float64, out map[string]float64) float64 {
	for _, p := range pays {
		var interest float64
		if p.Interest != nil {
			interest = math.Min(math.Max(*p.Interest, 0), p.Amount)
		} else {
			interest = math.Min(math.Max(due, 0), p.Amount)
		}
		due -= interest
		out[p.ID] = interest
	}
	return due
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
		if !p.Date.After(snaps[0].AsOf) {
			backward = append(backward, p)
		}
	}
	splitBackward(snaps[0], backward, out)

	for k, s := range snaps {
		var seg []MathPayment
		for _, p := range pays {
			if p.Date.After(s.AsOf) && (k == len(snaps)-1 || !p.Date.After(snaps[k+1].AsOf)) {
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
	r := snapshotRate(s)
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
		due := balance * r
		if accruedBefore && g.due.Equal(accrued) {
			due = 0
		}
		for _, p := range g.pays {
			due = allocate([]MathPayment{p}, due, out)
			balance -= p.Amount - out[p.ID]
		}
	}
}

func splitBackward(s MathSnapshot, seg []MathPayment, out map[string]float64) {
	if len(seg) == 0 {
		return
	}
	r := snapshotRate(s)
	groups := groupByWindow(s, seg)
	end := s.Balance
	for i := len(groups) - 1; i >= 0; i-- {
		var total float64
		for _, p := range groups[i].pays {
			total += p.Amount
		}
		start := (end + total) / (1 + r)
		due := start * r
		if total < due {
			due, start = total, end
		}
		allocate(groups[i].pays, due, out)
		end = start
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
			if p.Date.After(t) && !p.Date.After(snaps[0].AsOf) {
				balance += p.Amount - interest[p.ID]
			}
		}
		return balance
	}
	balance := snaps[idx].Balance
	for _, p := range pays {
		if p.Date.After(snaps[idx].AsOf) && !p.Date.After(t) {
			balance -= p.Amount - interest[p.ID]
		}
	}
	return balance
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

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
	extraTotal float64
	closing    map[string]float64
	endBalance float64
	payoffDate time.Time
}

func (r projRun) moneyLeft() float64 {
	total := r.extraTotal
	for _, s := range r.steps {
		total += s.pay
	}
	return total
}

// runProjection amortises balance forward from firstDue. An extra goes fully
// to principal right after the month's first projected payment (the regular
// payment already took that period's interest), or at month end when the
// month has no projected payment.
func runProjection(s MathSnapshot, rate, balance float64, today, firstDue time.Time, extras map[string]float64) projRun {
	run := projRun{closing: map[string]float64{}, endBalance: balance}
	if balance <= debtEpsilon {
		run.endBalance = 0
		return run
	}
	anchor := int16(debtDay(s.Last).Day())
	due := firstDue
	month := monthStart(today)
	for i := 0; i < maxProjectMonths && balance > debtEpsilon; i++ {
		next := month.AddDate(0, 1, 0)
		extra := extras[monthKey(month)]
		stepped := false
		for balance > debtEpsilon && due.Before(next) {
			interest := balance * rate
			pay := math.Min(s.Payment, balance+interest)
			balance = balance + interest - pay
			run.steps = append(run.steps, projStep{date: due, pay: pay})
			run.payoffDate = due
			if balance <= debtEpsilon {
				balance = 0
				break
			}
			if !stepped && extra > 0 {
				applied := math.Min(extra, balance)
				balance -= applied
				run.extraTotal += applied
				if balance <= debtEpsilon {
					balance = 0
					break
				}
			}
			stepped = true
			due = NextOccurrence(due, s.Schedule, anchor)
		}
		if !stepped && balance > debtEpsilon && extra > 0 {
			applied := math.Min(extra, balance)
			balance -= applied
			run.extraTotal += applied
			if balance <= debtEpsilon {
				balance = 0
				run.payoffDate = next.AddDate(0, 0, -1)
			}
		}
		run.closing[monthKey(month)] = balance
		month = next
	}
	run.endBalance = balance
	return run
}

// payoffEvent finds the event (a payment or a snapshot) that took the balance
// to zero, provided nothing was paid after it.
func payoffEvent(snaps []MathSnapshot, pays []MathPayment, interest map[string]float64, today time.Time) (time.Time, bool) {
	seen := map[time.Time]bool{}
	var times []time.Time
	for _, s := range snaps {
		if !s.AsOf.After(today) && !seen[s.AsOf] {
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
		if round2(BalanceAt(snaps, pays, interest, times[i])) > 0 {
			break
		}
		at, found = times[i], true
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
	rate := snapshotRate(latest)
	current := BalanceAt(snaps, pays, in.Interest, today)

	if at, ok := payoffEvent(snaps, pays, in.Interest, today); ok && round2(current) <= 0 {
		out.PaidOff, out.PayoffMonth = true, monthStart(at)
	}
	if current < 0 {
		current = 0
	}
	out.CurrentBalance = current
	for _, p := range pays {
		if !p.Date.After(today) {
			out.OverpaidSoFar += in.Interest[p.ID]
		}
	}

	firstDue := firstUnpaidDue(latest, pays, today)
	base := runProjection(latest, rate, current, today, firstDue, nil)
	currentMonth := monthStart(today)
	extras := map[string]float64{}
	for key, planned := range in.Plans {
		m, err := time.Parse(monthKeyLayout, key)
		if err != nil || m.Before(currentMonth) {
			continue
		}
		var covered float64
		if m.Equal(currentMonth) {
			covered = paidBetween(pays, currentMonth, today) + stillDueBefore(base, currentMonth.AddDate(0, 1, 0))
		} else {
			covered = latest.Payment * float64(len(DueDatesBetween(latest.Schedule, latest.Last, m.AddDate(0, 0, -1), m.AddDate(0, 1, -1))))
		}
		if extra := planned - covered; extra > 0 {
			extras[key] = extra
		}
	}
	run := runProjection(latest, rate, current, today, firstDue, extras)

	out.PaymentsLeft = len(run.steps)
	out.MoneyLeft = run.moneyLeft()
	out.OverpaymentLeft = out.MoneyLeft - current
	out.PayoffDate = run.payoffDate
	if len(run.steps) > 0 {
		out.NextDueDate = run.steps[0].date
	}
	out.BasePayoffDate = base.payoffDate
	out.BaseOverpaymentLeft = base.moneyLeft() - current

	for _, m := range in.Months {
		m = monthStart(m)
		monthEnd := m.AddDate(0, 1, -1)
		pm := ProjectionMonth{}
		pm.ScheduledTotal = latest.Payment * float64(len(DueDatesBetween(latest.Schedule, latest.Last, m.AddDate(0, 0, -1), monthEnd)))
		stepsReach := false
		for _, st := range run.steps {
			// An unpaid due before this month still has to be paid, so it
			// counts in the current month.
			date := st.date
			if date.Before(currentMonth) {
				date = currentMonth
			}
			if !date.Before(m) {
				stepsReach = true
			}
			switch {
			case date.After(monthEnd):
				pm.PaymentsLeft++
			case !date.Before(m):
				pm.StillDue += st.pay
			}
		}
		// From the current month on, nothing is scheduled once the projection
		// has paid the debt off, whatever the term says.
		if !m.Before(currentMonth) && !stepsReach {
			pm.ScheduledTotal = 0
		}
		switch {
		case m.Before(monthStart(today)):
			pm.ClosingBalance = math.Max(BalanceAt(snaps, pays, in.Interest, monthEnd), 0)
		default:
			closing, ok := run.closing[monthKey(m)]
			if !ok {
				closing = run.endBalance
			}
			pm.ClosingBalance = closing
		}
		out.Months[monthKey(m)] = pm
	}
	return out
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
func firstUnpaidDue(s MathSnapshot, pays []MathPayment, today time.Time) time.Time {
	anchor := int16(debtDay(s.Last).Day())
	due := dueContaining(s.Schedule, s.Last, today)
	if due.IsZero() {
		due = today
	}
	for i := 0; i < maxProjectMonths*5; i++ {
		if due.After(s.AsOf) && !paidWindow(s, pays, due, today) {
			break
		}
		due = NextOccurrence(due, s.Schedule, anchor)
	}
	return due
}

func paidWindow(s MathSnapshot, pays []MathPayment, due, today time.Time) bool {
	start, end := DueWindow(s.Schedule, s.Last, due)
	for _, p := range pays {
		if !p.Date.After(today) && !p.Date.Before(start) && p.Date.Before(end) {
			return true
		}
	}
	return false
}
