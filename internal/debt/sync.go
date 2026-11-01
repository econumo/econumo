package debt

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// payment is a matched expense in the debt's currency with its stored split.
type payment struct {
	txID     vo.Id
	date     time.Time
	amount   float64
	interest float64
}

type debtState struct {
	debt   *model.Debt
	view   model.DebtView
	snaps  []*model.DebtSnapshot
	pays   []payment // by date, then id
	digits int
}

func (st *debtState) mathSnaps() []model.MathSnapshot {
	out := make([]model.MathSnapshot, 0, len(st.snaps))
	for _, s := range st.snaps {
		out = append(out, model.MathSnapshot{AsOf: s.AsOf, Balance: toFloat(s.Balance), Payment: toFloat(s.Payment),
			Schedule: s.Schedule, Last: s.LastPaymentDate})
	}
	return out
}

func (st *debtState) mathPays() ([]model.MathPayment, map[string]float64) {
	out := make([]model.MathPayment, 0, len(st.pays))
	interest := make(map[string]float64, len(st.pays))
	for _, p := range st.pays {
		id := p.txID.String()
		out = append(out, model.MathPayment{ID: id, Date: p.date, Amount: p.amount})
		interest[id] = p.interest
	}
	return out, interest
}

func (st *debtState) project(today time.Time, months []time.Time, plans map[string]float64) model.Projection {
	pays, interest := st.mathPays()
	return model.ProjectDebt(model.ProjectionInput{Snaps: st.mathSnaps(), Pays: pays, Interest: interest,
		Today: today, Months: months, Plans: plans})
}

// Sync brings the stored payment splits of the owners' debts up to date.
func (s *Service) Sync(ctx context.Context, ownerIDs []vo.Id) error {
	_, err := s.loadStates(ctx, ownerIDs)
	return err
}

// loadStates syncs every category the owners have debts in and returns the
// debts' states in creation order.
func (s *Service) loadStates(ctx context.Context, ownerIDs []vo.Id) ([]*debtState, error) {
	debts, err := s.repo.ListByOwners(ctx, ownerIDs)
	if err != nil {
		return nil, err
	}
	var categories []vo.Id
	seen := map[vo.Id]bool{}
	for _, d := range debts {
		if !seen[d.CategoryID] {
			seen[d.CategoryID] = true
			categories = append(categories, d.CategoryID)
		}
	}
	var out []*debtState
	for _, c := range categories {
		states, err := s.syncCategory(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, states...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].debt, out[j].debt
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID.String() < b.ID.String()
	})
	return out, nil
}

// matchViews assigns every debt its matching key and range. Debts sharing a
// key are partitioned by start date: each matches from its own start up to
// the next one's.
func matchViews(debts []*model.Debt) map[vo.Id]model.DebtView {
	type key struct{ payee, tag string }
	groups := map[key][]*model.Debt{}
	for _, d := range debts {
		k := key{}
		if d.PayeeID != nil {
			k.payee = d.PayeeID.String()
		}
		if d.TagID != nil {
			k.tag = d.TagID.String()
		}
		groups[k] = append(groups[k], d)
	}
	views := make(map[vo.Id]model.DebtView, len(debts))
	for _, g := range groups {
		sort.SliceStable(g, func(i, j int) bool {
			a, b := g[i].StartDate, g[j].StartDate
			switch {
			case a == nil && b == nil:
				return g[i].ID.String() < g[j].ID.String()
			case a == nil:
				return true
			case b == nil:
				return false
			}
			return a.Before(*b)
		})
		for i, d := range g {
			v := model.DebtView{
				ID: d.ID.String(), OwnerID: d.OwnerID.String(), CurrencyID: d.CurrencyID.String(),
				CategoryID: d.CategoryID.String(), Name: d.Name, WholeCategory: isWholeCategory(d),
			}
			if d.PayeeID != nil {
				p := d.PayeeID.String()
				v.PayeeID = &p
			}
			if d.TagID != nil {
				t := d.TagID.String()
				v.TagID = &t
			}
			if d.StartDate != nil {
				from := *d.StartDate
				v.From = &from
			}
			if i+1 < len(g) && g[i+1].StartDate != nil {
				until := *g[i+1].StartDate
				v.Until = &until
			}
			views[d.ID] = v
		}
	}
	return views
}

// syncCategory matches the category's expenses to its debts, drops splits
// that no longer belong and splits the new payments, then returns the debts'
// states. It runs in one transaction under the owner's row lock.
func (s *Service) syncCategory(ctx context.Context, categoryID vo.Id) ([]*debtState, error) {
	var states []*debtState
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		first, err := s.repo.ListByCategory(ctx, categoryID)
		if err != nil || len(first) == 0 {
			return err
		}
		if err := s.repo.LockOwner(ctx, first[0].OwnerID); err != nil {
			return err
		}
		debts, err := s.repo.ListByCategory(ctx, categoryID)
		if err != nil {
			return err
		}
		states, err = s.syncDebts(ctx, categoryID, debts)
		return err
	})
	if err != nil {
		return nil, err
	}
	return states, nil
}

func (s *Service) syncDebts(ctx context.Context, categoryID vo.Id, debts []*model.Debt) ([]*debtState, error) {
	if len(debts) == 0 {
		return nil, nil
	}
	ids := make([]vo.Id, 0, len(debts))
	for _, d := range debts {
		ids = append(ids, d.ID)
	}
	snaps, err := s.repo.Snapshots(ctx, ids)
	if err != nil {
		return nil, err
	}
	splits, err := s.repo.Splits(ctx, ids)
	if err != nil {
		return nil, err
	}
	cands, err := s.repo.CandidateTransactions(ctx, categoryID)
	if err != nil {
		return nil, err
	}

	views := matchViews(debts)
	cat := categoryID.String()
	assigned := map[vo.Id][]CandidateTx{}
	matchedTo := map[vo.Id]vo.Id{}
	for _, c := range cands {
		var payee, tag *string
		if c.PayeeID != nil {
			p := c.PayeeID.String()
			payee = &p
		}
		if c.TagID != nil {
			t := c.TagID.String()
			tag = &t
		}
		for _, d := range debts {
			if model.DebtMatches(views[d.ID], &cat, payee, tag, c.SpentAt) {
				assigned[d.ID] = append(assigned[d.ID], c)
				matchedTo[c.ID] = d.ID
				break
			}
		}
	}

	stored := map[vo.Id]vo.DecimalNumber{}
	stale := map[vo.Id][]vo.Id{}
	for _, sp := range splits {
		if matchedTo[sp.TransactionID] == sp.DebtID {
			stored[sp.TransactionID] = sp.Interest
			continue
		}
		stale[sp.DebtID] = append(stale[sp.DebtID], sp.TransactionID)
	}
	for _, d := range debts {
		if err := s.repo.DeleteSplits(ctx, d.ID, stale[d.ID]); err != nil {
			return nil, err
		}
	}

	snapsByDebt := map[vo.Id][]*model.DebtSnapshot{}
	for _, sn := range snaps {
		snapsByDebt[sn.DebtID] = append(snapsByDebt[sn.DebtID], sn)
	}
	digitsByCurrency := map[vo.Id]int{}
	states := make([]*debtState, 0, len(debts))
	for _, d := range debts {
		digits, ok := digitsByCurrency[d.CurrencyID]
		if !ok {
			if digits, err = s.rates.FractionDigits(ctx, d.CurrencyID); err != nil {
				return nil, err
			}
			digitsByCurrency[d.CurrencyID] = digits
		}
		st := &debtState{debt: d, view: views[d.ID], snaps: snapsByDebt[d.ID], digits: digits}
		if err := s.fillPayments(ctx, st, assigned[d.ID], stored); err != nil {
			return nil, err
		}
		states = append(states, st)
	}
	return states, nil
}

// fillPayments converts the debt's matched expenses and stores the split of
// those without one. Stored interests are fed to the split as they are, so
// payments split earlier never change.
func (s *Service) fillPayments(ctx context.Context, st *debtState, cands []CandidateTx, stored map[vo.Id]vo.DecimalNumber) error {
	d := st.debt
	sort.SliceStable(cands, func(i, j int) bool {
		if !cands[i].SpentAt.Equal(cands[j].SpentAt) {
			return cands[i].SpentAt.Before(cands[j].SpentAt)
		}
		return cands[i].ID.String() < cands[j].ID.String()
	})
	pays := make([]payment, 0, len(cands))
	var missing bool
	for _, c := range cands {
		day := time.Date(c.SpentAt.Year(), c.SpentAt.Month(), c.SpentAt.Day(), 0, 0, 0, 0, time.UTC)
		amount := c.Amount
		if !c.CurrencyID.Equal(d.CurrencyID) {
			converted, err := s.rates.ConvertOnDay(ctx, c.CurrencyID, d.CurrencyID, c.Amount, day)
			if err != nil {
				return err
			}
			amount = converted
		}
		p := payment{txID: c.ID, date: day, amount: roundTo(toFloat(amount), st.digits)}
		if interest, ok := stored[c.ID]; ok {
			p.interest = toFloat(interest)
		} else {
			missing = true
		}
		pays = append(pays, p)
	}
	st.pays = pays
	if !missing || len(st.snaps) == 0 {
		return nil
	}

	mathPays := make([]model.MathPayment, 0, len(pays))
	for _, p := range pays {
		mp := model.MathPayment{ID: p.txID.String(), Date: p.date, Amount: p.amount}
		if _, ok := stored[p.txID]; ok {
			interest := p.interest
			mp.Interest = &interest
		}
		mathPays = append(mathPays, mp)
	}
	split := model.SplitPayments(st.mathSnaps(), mathPays)
	for i := range st.pays {
		p := &st.pays[i]
		if _, ok := stored[p.txID]; ok {
			continue
		}
		p.interest = roundTo(split[p.txID.String()], st.digits)
		err := s.repo.InsertSplit(ctx, model.DebtSplit{DebtID: d.ID, TransactionID: p.txID,
			Interest: vo.NewDecimal(strconv.FormatFloat(p.interest, 'f', st.digits, 64))})
		if err != nil {
			return err
		}
	}
	return nil
}
