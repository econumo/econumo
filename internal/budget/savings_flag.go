package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// memberChange is one write's effect on a budget's member accounts and their
// savings flags. It is worked out in full before anything is written, so the
// savings guard can refuse the whole request and leave no trace.
type memberChange struct {
	budgetID vo.Id
	current  map[string]bool // member -> savings flag before the write
	remove   []vo.Id
	add      []memberFlag
	flags    []memberFlag // existing members whose flag changes
}

type memberFlag struct {
	id vo.Id
	on bool
}

func newMemberChange(budgetID vo.Id, members []model.BudgetAccount) *memberChange {
	c := &memberChange{budgetID: budgetID, current: make(map[string]bool, len(members))}
	for _, m := range members {
		c.current[m.AccountID.String()] = m.IsSavings
	}
	return c
}

func (c *memberChange) addMember(id vo.Id, isSavings bool) {
	c.add = append(c.add, memberFlag{id: id, on: isSavings})
}

func (c *memberChange) removeMember(id vo.Id) { c.remove = append(c.remove, id) }

func (c *memberChange) setFlag(id vo.Id, on bool) {
	if c.current[id.String()] != on {
		c.flags = append(c.flags, memberFlag{id: id, on: on})
	}
}

func (c *memberChange) empty() bool {
	return len(c.remove) == 0 && len(c.add) == 0 && len(c.flags) == 0
}

// leaving lists the savings members that stop being savings rows: flag turned
// off, or membership removed.
func (c *memberChange) leaving() []vo.Id {
	var out []vo.Id
	for _, id := range c.remove {
		if c.current[id.String()] {
			out = append(out, id)
		}
	}
	for _, f := range c.flags {
		if !f.on {
			out = append(out, f.id)
		}
	}
	return out
}

// touchesSavings reports whether a savings row appears or disappears.
func (c *memberChange) touchesSavings() bool {
	if len(c.leaving()) > 0 {
		return true
	}
	for _, a := range c.add {
		if a.on {
			return true
		}
	}
	for _, f := range c.flags {
		if f.on {
			return true
		}
	}
	return false
}

// guardSavingsRemoval refuses, unless confirmed, a write that would delete a
// savings row still carrying planned amounts or comments. It runs on the
// server so it holds whatever the client had loaded.
func (s *Service) guardSavingsRemoval(ctx context.Context, c *memberChange, confirmed bool) error {
	if confirmed {
		return nil
	}
	for _, id := range c.leaving() {
		has, err := s.budgets.SavingsElementHasData(ctx, c.budgetID, id)
		if err != nil {
			return err
		}
		if has {
			return savingsRemovalUnconfirmed()
		}
	}
	return nil
}

// applyMemberChange writes the change and, when the savings set moved, syncs
// the elements in the same transaction so a savings row appears or goes (with
// its limits and comments, by cascade) together with its flag.
func (s *Service) applyMemberChange(ctx context.Context, c *memberChange, now time.Time) error {
	for _, id := range c.remove {
		if err := s.budgets.RemoveAccount(ctx, c.budgetID, id); err != nil {
			return err
		}
	}
	for _, a := range c.add {
		if err := s.budgets.AddAccount(ctx, c.budgetID, a.id, a.on, now); err != nil {
			return err
		}
	}
	for _, f := range c.flags {
		if err := s.budgets.SetAccountSavings(ctx, c.budgetID, f.id, f.on); err != nil {
			return err
		}
	}
	if c.touchesSavings() {
		return s.syncElements(ctx, c.budgetID, now)
	}
	return nil
}

// writeMemberChange is plan + guard + apply in one transaction, for the use
// cases whose only write is the membership change. The plan works from the
// members read inside the transaction, not the aggregate loaded before it: a
// member another request flagged and planned in between must still meet the
// guard, and its savings row must still go with it.
func (s *Service) writeMemberChange(ctx context.Context, budgetID vo.Id, confirmed bool, plan func(ctx context.Context, c *memberChange) error) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		members, err := s.budgets.MemberAccounts(txCtx, budgetID)
		if err != nil {
			return err
		}
		c := newMemberChange(budgetID, members)
		if err := plan(txCtx, c); err != nil {
			return err
		}
		if c.empty() {
			return nil
		}
		if err := s.guardSavingsRemoval(txCtx, c, confirmed); err != nil {
			return err
		}
		return s.applyMemberChange(txCtx, c, s.clock.Now())
	})
}

func (c *memberChange) isMember(id vo.Id) bool {
	_, ok := c.current[id.String()]
	return ok
}

// planOwnMembership works out update-budget's change over the CALLER's own
// member accounts; other participants' members and flags are never touched.
// accountIDs (nil = untouched) replaces the caller's members, but a member with
// closed-month history is permanent, so naming a set that drops one fails.
// savingsIDs (nil = untouched) replaces the caller's savings members, checked
// against the membership accountIDs leaves.
func (s *Service) planOwnMembership(ctx context.Context, userID vo.Id, b *budgetAggregate, accountIDs, savingsIDs []string, now time.Time) (*memberChange, error) {
	c := newMemberChange(b.budget.ID, b.accounts)
	if accountIDs == nil && savingsIDs == nil {
		return c, nil
	}
	var own []vo.Id
	for _, m := range b.accounts {
		owned, err := s.ownsAccount(ctx, userID, m.AccountID)
		if err != nil {
			return nil, err
		}
		if owned {
			own = append(own, m.AccountID)
		}
	}
	after := make(map[string]bool, len(own))
	for _, m := range own {
		after[m.String()] = true
	}

	if accountIDs != nil {
		want := map[string]bool{}
		var wantOrder []vo.Id
		for _, raw := range accountIDs {
			aid, err := vo.ParseId(raw)
			if err != nil {
				return nil, model.ValidateBlank(map[string]string{"accountIds": ""})
			}
			owned, err := s.ownsAccount(ctx, userID, aid)
			if err != nil {
				return nil, err
			}
			if !owned || want[aid.String()] {
				continue
			}
			want[aid.String()] = true
			wantOrder = append(wantOrder, aid)
		}
		removable, err := s.removableAccounts(ctx, b, own, now)
		if err != nil {
			return nil, err
		}
		for _, m := range own {
			if want[m.String()] {
				continue
			}
			if !removable[m.String()] {
				return nil, accountNotRemovable()
			}
			c.removeMember(m)
			delete(after, m.String())
		}
		for _, aid := range wantOrder {
			// Naming an existing member again is a no-op. Deleted members stay
			// listed in the filters block (they keep counting), so a client
			// round-tripping that list back names them — rejecting the id would
			// wedge every later update, since the removal rule keeps such a
			// member forever. Only a NEW member has to be a live account.
			if _, member := c.current[aid.String()]; member {
				continue
			}
			views, err := s.accounts.AccountsByIDs(ctx, []vo.Id{aid})
			if err != nil {
				return nil, err
			}
			if views[0].IsDeleted {
				return nil, model.ValidateBlank(map[string]string{"accountIds": ""})
			}
			c.addMember(aid, false)
			after[aid.String()] = true
		}
	}

	if savingsIDs != nil {
		target := map[string]bool{}
		for _, raw := range savingsIDs {
			aid, err := vo.ParseId(raw)
			if err != nil || !after[aid.String()] {
				return nil, savingsAccountNotMember()
			}
			target[aid.String()] = true
		}
		for i := range c.add {
			c.add[i].on = target[c.add[i].id.String()]
		}
		for _, m := range own {
			if after[m.String()] {
				c.setFlag(m, target[m.String()])
			}
		}
	}
	return c, nil
}

// savingsSubsetOf checks create-budget's savingsAccountIds against its
// accountIds before anything is written.
func savingsSubsetOf(accountIDs, savingsIDs []string) (map[string]bool, error) {
	members := make(map[string]bool, len(accountIDs))
	for _, raw := range accountIDs {
		if aid, err := vo.ParseId(raw); err == nil {
			members[aid.String()] = true
		}
	}
	out := make(map[string]bool, len(savingsIDs))
	for _, raw := range savingsIDs {
		aid, err := vo.ParseId(raw)
		if err != nil || !members[aid.String()] {
			return nil, savingsAccountNotMember()
		}
		out[aid.String()] = true
	}
	return out, nil
}

func savingsAccountNotMember() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key:     "savingsAccountIds",
		Message: "Savings accounts must be your own accounts in this budget",
		Code:    errs.CodeBudgetSavingsAccountNotMember,
	})
}

// The field key is the SPA's handle on this refusal: the envelope carries no
// catalogue code, so the client recognises the error by field.
func savingsRemovalUnconfirmed() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key:     "confirmSavingsRemoval",
		Message: "This deletes the planned amounts and comments of savings accounts you turned off or removed from this budget; confirm to continue",
		Code:    errs.CodeBudgetSavingsRemovalUnconfirmed,
	})
}
