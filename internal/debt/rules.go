package debt

import (
	"context"
	"slices"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const maxRules = 10

// ruleInput is a rule of a request: its key, and the id it names (nil when
// none).
type ruleInput struct {
	id  *vo.Id
	key model.DebtRule
}

func parseRule(r model.DebtRuleRequest) (ruleInput, error) {
	var in ruleInput
	var err error
	if in.id, err = parseOptionalID(r.Id); err != nil {
		return in, err
	}
	if in.key.CategoryID, err = vo.ParseId(r.CategoryId); err != nil {
		return in, err
	}
	if in.key.PayeeID, err = parseOptionalID(r.PayeeId); err != nil {
		return in, err
	}
	if in.key.TagID, err = parseOptionalID(r.TagId); err != nil {
		return in, err
	}
	in.key.LabelID, err = parseOptionalID(r.LabelId)
	return in, err
}

func parseRules(rs []model.DebtRuleRequest) ([]ruleInput, error) {
	out := make([]ruleInput, 0, len(rs))
	for _, r := range rs {
		in, err := parseRule(r)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, nil
}

func (s *Service) mint(in ruleInput, now time.Time) model.DebtRule {
	r := in.key
	r.ID, r.CreatedAt = s.repo.NextIdentity(), now
	return r
}

// mergeRules replaces cur with the requested rules: a rule matched by its id,
// else by an identical key, keeps its id and creation time (by id even when
// its key changed); any other is new. fresh are the new and changed rules,
// the only ones validated again: a kept rule of a participant who has since
// left must not block an edit of the debt.
func (s *Service) mergeRules(cur []model.DebtRule, req []ruleInput, now time.Time) (rules, fresh []model.DebtRule) {
	used := make([]bool, len(cur))
	match := func(in ruleInput) (int, bool) {
		if in.id != nil {
			for i, r := range cur {
				if !used[i] && r.ID.Equal(*in.id) {
					return i, true
				}
			}
		}
		for i, r := range cur {
			if !used[i] && r.SameKey(in.key) {
				return i, true
			}
		}
		return -1, false
	}
	for _, in := range req {
		i, ok := match(in)
		if !ok {
			r := s.mint(in, now)
			rules, fresh = append(rules, r), append(fresh, r)
			continue
		}
		used[i] = true
		r := in.key
		r.ID, r.CreatedAt = cur[i].ID, cur[i].CreatedAt
		rules = append(rules, r)
		if !r.SameKey(cur[i]) {
			fresh = append(fresh, r)
		}
	}
	sortRules(rules)
	return rules, fresh
}

func sortRules(rules []model.DebtRule) {
	slices.SortStableFunc(rules, func(a, b model.DebtRule) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return compareIDs(a.ID, b.ID)
	})
}

func compareIDs(a, b vo.Id) int {
	switch as, bs := a.String(), b.String(); {
	case as < bs:
		return -1
	case as > bs:
		return 1
	}
	return 0
}

// checkRuleOwners: a rule's category is an expense category of a participant
// of the budget (the owner or an accepted non-guest, whose categories the
// budget shows), and its payee, tag and label are that participant's.
func (s *Service) checkRuleOwners(ctx context.Context, b model.DebtBudget, r model.DebtRule) error {
	cat, err := s.class.Classification(ctx, KindCategory, r.CategoryID)
	if err != nil {
		if isNotFound(err) {
			return errs.NewNotFound("Category not found")
		}
		return err
	}
	if !slices.ContainsFunc(b.ParticipantIDs, cat.OwnerID.Equal) {
		return errs.NewNotFound("Category not found")
	}
	if !cat.IsExpense {
		return fieldError("rules", "A debt needs an expense category.", errs.CodeDebtCategoryNotExpense)
	}
	for _, part := range []struct {
		kind, missing string
		id            *vo.Id
	}{{KindPayee, "Payee not found", r.PayeeID}, {KindTag, "Tag not found", r.TagID}, {KindLabel, "Label not found", r.LabelID}} {
		if part.id == nil {
			continue
		}
		c, err := s.class.Classification(ctx, part.kind, *part.id)
		if err != nil && !isNotFound(err) {
			return err
		}
		if err != nil || !c.OwnerID.Equal(cat.OwnerID) {
			return errs.NewNotFound(part.missing)
		}
	}
	return nil
}

// checkRules refuses the fresh rules of d overlapping another rule of d, and
// any rule of d conflicting with another debt of the budget (an identical key
// on the same start date included, which a start change can cause). Two kept
// rules are not checked against each other: a classification cleanup may
// leave two overlapping rules in one debt.
func checkRules(d *model.Debt, fresh []model.DebtRule, budgetDebts []*model.Debt) error {
	for _, r := range fresh {
		for _, o := range d.Rules {
			if o.ID.Equal(r.ID) {
				continue
			}
			if err := model.CheckDebtRules(&model.Debt{ID: d.ID, Rules: []model.DebtRule{r, o}}, nil); err != nil {
				return err
			}
		}
	}
	for _, r := range d.Rules {
		probe := &model.Debt{ID: d.ID, StartDate: d.StartDate, Rules: []model.DebtRule{r}}
		if err := model.CheckDebtRules(probe, budgetDebts); err != nil {
			return err
		}
	}
	return nil
}

func ruleCategories(rules []model.DebtRule) []vo.Id {
	var out []vo.Id
	for _, r := range rules {
		if !slices.ContainsFunc(out, r.CategoryID.Equal) {
			out = append(out, r.CategoryID)
		}
	}
	return out
}

// writeRules validates the fresh rules of d in its budget. Their categories
// are key-share-locked first, so the owner check reads rows a concurrent
// delete can no longer remove: that delete waits for this write and then
// releases the rule. A category deleted before the lock is not found.
func (s *Service) writeRules(ctx context.Context, b model.DebtBudget, d *model.Debt, fresh []model.DebtRule, budgetDebts []*model.Debt) error {
	if err := s.repo.LockRuleCategories(ctx, ruleCategories(fresh)); err != nil {
		return err
	}
	for _, r := range fresh {
		if err := s.checkRuleOwners(ctx, b, r); err != nil {
			return err
		}
	}
	return checkRules(d, fresh, budgetDebts)
}

func tooManyRules() error {
	return fieldError("rules", "This value is out of range.", errs.CodeOutOfRange)
}

// AddDebtRule adds a rule to a debt ("Add to a debt"). A whole-category rule
// takes its category row's plans and comments onto the debt's row, adding up
// with what is there.
func (s *Service) AddDebtRule(ctx context.Context, userID vo.Id, req model.AddDebtRuleRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	in, err := parseRule(req.Rule)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	var saved *model.Debt
	var budget model.DebtBudget
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		cur, b, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		budget = b
		if len(cur.Rules) >= maxRules {
			return tooManyRules()
		}
		rule := s.mint(in, now)
		budgetDebts, err := s.repo.ListByBudget(ctx, cur.BudgetID)
		if err != nil {
			return err
		}
		cur.Rules = append(cur.Rules, rule)
		if err := s.writeRules(ctx, b, cur, []model.DebtRule{rule}, budgetDebts); err != nil {
			return err
		}
		cur.UpdatedAt = now
		if err := s.repo.Save(ctx, cur); err != nil {
			return err
		}
		saved = cur
		if model.IsWholeCategoryRule(cur, rule, budgetDebts) {
			return s.moveToDebt(ctx, cur, rule.CategoryID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, budget, saved)
}
