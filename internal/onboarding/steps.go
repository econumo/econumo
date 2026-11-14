package onboarding

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Step names as stored in the onboarding option.
const (
	StepCategories = "categories"
	StepAccounts   = "accounts"
	StepPlan       = "plan"
)

// begin opens a step inside its transaction: it takes the user row lock, then
// decides whether the step still has work to do. A step already done (also a
// replay whose first response was lost: that call marked it) has none, and the
// caller answers with the current state and writes nothing. An operation id
// already claimed while the step is open was used by another write and is
// refused.
func (s *Service) begin(ctx context.Context, userID vo.Id, rawOpID, step string) (*model.CurrentUserResult, bool, error) {
	user, err := s.steps.Users.LockForOnboarding(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if model.OnboardingStepDone(optionValue(user, model.OptionOnboarding), step) {
		return user, false, nil
	}
	opID, err := vo.ParseId(strings.TrimSpace(rawOpID))
	if err != nil {
		return nil, false, err
	}
	already, err := s.steps.Ops.Claim(ctx, opID, s.steps.Clock.Now())
	if err != nil {
		return nil, false, err
	}
	if already {
		// The step is not done, so the id was claimed by another write: the
		// operation id table is shared by every create endpoint.
		return nil, false, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "id", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
		})
	}
	return user, true, nil
}

// finish marks the step done and the operation handled.
func (s *Service) finish(ctx context.Context, userID vo.Id, rawOpID, step string) (*model.CurrentUserResult, error) {
	opID, err := vo.ParseId(strings.TrimSpace(rawOpID))
	if err != nil {
		return nil, err
	}
	if err := s.steps.Ops.MarkHandled(ctx, opID, s.steps.Clock.Now()); err != nil {
		return nil, err
	}
	return s.steps.Users.MarkOnboardingStep(ctx, userID, step)
}

func optionValue(user *model.CurrentUserResult, name string) *string {
	for _, o := range user.Options {
		if o.Name == name {
			return o.Value
		}
	}
	return nil
}

func preferredBudget(user *model.CurrentUserResult) string {
	if v := optionValue(user, model.OptionBudget); v != nil {
		return *v
	}
	return ""
}

func nameKey(typ, name string) string {
	return typ + ":" + strings.ToLower(strings.TrimSpace(name))
}

// defaultBudgetID is the budget id a step answers with: "" when the user has none.
func (s *Service) defaultBudgetID(ctx context.Context, userID vo.Id, user *model.CurrentUserResult) (string, error) {
	id, _, found, err := s.steps.Budgets.DefaultBudget(ctx, userID, preferredBudget(user))
	if err != nil || !found {
		return "", err
	}
	return id.String(), nil
}

// createBudget creates the onboarding budget over accountIDs: named in the
// caller's language, in the profile currency, starting this period.
func (s *Service) createBudget(ctx context.Context, userID vo.Id, accountIDs []string) (string, error) {
	id := vo.NewId()
	_, err := s.steps.Budgets.CreateBudget(ctx, userID, model.CreateBudgetRequest{
		Id:         id.String(),
		Name:       i18n.T(reqctx.Language(ctx), "onboarding.budget_name", nil),
		AccountIds: accountIDs,
	})
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// ownVisibleAccountIDs are the user's own live accounts outside hidden
// folders: the accounts a budget onboarding creates starts with.
func (s *Service) ownVisibleAccountIDs(ctx context.Context, userID vo.Id) ([]string, error) {
	accounts, err := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(accounts))
	for _, a := range accounts {
		if !a.Hidden {
			ids = append(ids, a.ID)
		}
	}
	return ids, nil
}

func invalidChoice(key string) error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key: key, Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
	})
}

// SaveCategories creates the categories step's rows, applies the user's order,
// creates the budget when the user has accounts but no budget yet, and marks
// the step done, all in one transaction. A row named like one of the user's
// active categories of its type (or like an earlier row) maps to that category
// instead of creating a second one.
func (s *Service) SaveCategories(ctx context.Context, userID vo.Id, req model.OnboardingSaveCategoriesRequest) (*model.OnboardingSaveCategoriesResult, error) {
	out := &model.OnboardingSaveCategoriesResult{Categories: map[string]string{}}
	err := s.steps.Tx.WithTx(ctx, func(ctx context.Context) error {
		user, proceed, err := s.begin(ctx, userID, req.Id, StepCategories)
		if err != nil {
			return err
		}
		own, err := s.steps.Categories.OwnCategories(ctx, userID)
		if err != nil {
			return err
		}
		if !proceed {
			out.User = *user
			out.Categories = matchCategories(req.Categories, own)
			out.BudgetId, err = s.defaultBudgetID(ctx, userID, user)
			return err
		}
		known := map[string]string{}
		for _, c := range own {
			if c.IsArchived == 0 {
				if _, dup := known[nameKey(c.Type, c.Name)]; !dup {
					known[nameKey(c.Type, c.Name)] = c.Id
				}
			}
		}
		for _, row := range req.Categories {
			name := strings.TrimSpace(row.Name)
			if id, ok := known[nameKey(row.Type, name)]; ok {
				out.Categories[row.Key] = id
				continue
			}
			icon := strings.TrimSpace(row.Icon)
			res, cerr := s.steps.Categories.CreateCategory(ctx, userID, model.CreateCategoryRequest{
				Id: vo.NewId().String(), Name: name, Type: row.Type, Icon: &icon,
			})
			if cerr != nil {
				return cerr
			}
			out.Categories[row.Key] = res.Item.Id
			known[nameKey(row.Type, name)] = res.Item.Id
		}
		if len(req.Order) > 0 {
			wanted := make([]string, 0, len(req.Order))
			for _, k := range req.Order {
				if id, ok := out.Categories[k]; ok {
					wanted = append(wanted, id)
				} else {
					wanted = append(wanted, k)
				}
			}
			if oerr := s.applyCategoryOrder(ctx, userID, wanted); oerr != nil {
				return oerr
			}
		}
		if aerr := s.archiveCategories(ctx, userID, own, req.Archive, req.Restore); aerr != nil {
			return aerr
		}
		out.BudgetId, err = s.defaultBudgetID(ctx, userID, user)
		if err != nil {
			return err
		}
		if out.BudgetId == "" {
			accounts, aerr := s.ownVisibleAccountIDs(ctx, userID)
			if aerr != nil {
				return aerr
			}
			if len(accounts) > 0 {
				if out.BudgetId, err = s.createBudget(ctx, userID, accounts); err != nil {
					return err
				}
			}
		}
		done, err := s.finish(ctx, userID, req.Id, StepCategories)
		if err != nil {
			return err
		}
		out.User = *done
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// archiveCategories archives and restores the named categories, which must be
// the user's own (else a 400 on the list naming them). A category already in
// the wanted state is left alone.
func (s *Service) archiveCategories(ctx context.Context, userID vo.Id, own []model.CategoryResult, archive, restore []string) error {
	archived := map[string]bool{}
	for _, c := range own {
		archived[strings.ToLower(c.Id)] = c.IsArchived == 1
	}
	for _, list := range []struct {
		key  string
		ids  []string
		want bool
	}{{"archive", archive, true}, {"restore", restore, false}} {
		for _, raw := range list.ids {
			id := strings.ToLower(strings.TrimSpace(raw))
			state, mine := archived[id]
			if !mine {
				return invalidChoice(list.key)
			}
			if state == list.want {
				continue
			}
			var err error
			if list.want {
				_, err = s.steps.Categories.ArchiveCategory(ctx, userID, model.ArchiveCategoryRequest{Id: id})
			} else {
				_, err = s.steps.Categories.UnarchiveCategory(ctx, userID, model.UnarchiveCategoryRequest{Id: id})
			}
			if err != nil {
				return err
			}
			archived[id] = list.want
		}
	}
	return nil
}

// matchCategories recovers the key->id map of an earlier call by name among the
// user's categories, active ones first.
func matchCategories(rows []model.OnboardingCategoryRow, own []model.CategoryResult) map[string]string {
	byName := map[string]string{}
	for _, archived := range []int{0, 1} {
		for _, c := range own {
			k := nameKey(c.Type, c.Name)
			if _, ok := byName[k]; !ok && c.IsArchived == archived {
				byName[k] = c.Id
			}
		}
	}
	out := map[string]string{}
	for _, r := range rows {
		if id, ok := byName[nameKey(r.Type, r.Name)]; ok {
			out[r.Key] = id
		}
	}
	return out
}

// applyCategoryOrder gives the wanted ids the slots their type's rows hold in
// the user's list, each type ordered on its own; archived and unnamed rows keep
// their place. An order already in place writes nothing.
func (s *Service) applyCategoryOrder(ctx context.Context, userID vo.Id, wanted []string) error {
	own, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		return err
	}
	typeOf := map[string]string{}
	for _, c := range own {
		typeOf[c.Id] = c.Type
	}
	queue := map[string][]string{}
	placed := map[string]bool{}
	for _, id := range wanted {
		if t, ok := typeOf[id]; ok && !placed[id] {
			queue[t] = append(queue[t], id)
			placed[id] = true
		}
	}
	next := make([]string, 0, len(own))
	changed := false
	for _, c := range own {
		id := c.Id
		if placed[id] && len(queue[c.Type]) > 0 {
			id, queue[c.Type] = queue[c.Type][0], queue[c.Type][1:]
		}
		changed = changed || id != c.Id
		next = append(next, id)
	}
	if !changed {
		return nil
	}
	_, err = s.steps.Categories.SortCategoryList(ctx, userID, model.SortCategoryListRequest{Ids: next})
	return err
}

// SaveAccounts creates the accounts step's rows, sets the profile currency and
// the visible currencies, then puts the new accounts in a budget: a new one
// over all the user's live accounts when they have none, else their default
// budget. The step is marked done in the same transaction. After the commit,
// when an AI provider is configured, the new accounts are classified as savings
// or not in the background.
func (s *Service) SaveAccounts(ctx context.Context, userID vo.Id, req model.OnboardingSaveAccountsRequest) (*model.OnboardingSaveAccountsResult, error) {
	out := &model.OnboardingSaveAccountsResult{Accounts: map[string]string{}}
	var classify []vo.Id
	var classifyBudget vo.Id
	err := s.steps.Tx.WithTx(ctx, func(ctx context.Context) error {
		classify = nil
		user, proceed, err := s.begin(ctx, userID, req.Id, StepAccounts)
		if err != nil {
			return err
		}
		if !proceed {
			own, aerr := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
			if aerr != nil {
				return aerr
			}
			out.User = *user
			out.Accounts = matchAccounts(req.Accounts, own)
			out.BudgetId, err = s.defaultBudgetID(ctx, userID, user)
			return err
		}
		mainID := strings.TrimSpace(req.MainCurrencyId)
		mainCode, err := s.steps.Currencies.UsableCurrencyCode(ctx, userID, mainID)
		if err != nil {
			if refusal(err) {
				return errs.NewValidation("Validation failed", errs.FieldError{
					Key: "mainCurrencyId", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
				})
			}
			return err
		}
		folderID := ""
		if len(req.Accounts) > 0 {
			if folderID, err = s.visibleFolderID(ctx, userID, false); err != nil {
				return err
			}
		}
		created := make([]vo.Id, 0, len(req.Accounts))
		for _, row := range req.Accounts {
			balance := strings.TrimSpace(row.Balance.String())
			if balance == "" {
				balance = "0"
			}
			res, cerr := s.steps.Accounts.CreateAccount(ctx, userID, model.CreateAccountRequest{
				Id:         vo.NewId().String(),
				Name:       strings.TrimSpace(row.Name),
				CurrencyId: strings.TrimSpace(row.CurrencyId),
				Balance:    vo.NewFlexString(balance),
				Icon:       row.Icon,
				FolderId:   folderID,
			})
			if cerr != nil {
				return cerr
			}
			// The first account of a user with no folder creates "General"; the rest join it.
			if folderID == "" && res.Item.FolderId != nil {
				folderID = *res.Item.FolderId
			}
			out.Accounts[row.Key] = res.Item.Id
			created = append(created, vo.MustParseId(res.Item.Id))
		}

		if cerr := s.reviseAccounts(ctx, userID, req.Currencies, req.Balances); cerr != nil {
			return cerr
		}
		if _, err := s.steps.Users.UpdateCurrency(ctx, userID, model.UpdateCurrencyRequest{Currency: mainCode}); err != nil {
			return err
		}
		own, err := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
		if err != nil {
			return err
		}
		if err := s.showOnly(ctx, userID, visibleCurrencies(mainID, req, own)); err != nil {
			return err
		}
		hidden, unhidden, err := s.moveAccounts(ctx, userID, own, req.Hide, req.Unhide)
		if err != nil {
			return err
		}

		budgetID, writable, found, err := s.steps.Budgets.DefaultBudget(ctx, userID, preferredBudget(user))
		if err != nil {
			return err
		}
		switch {
		case !found:
			accounts, aerr := s.ownVisibleAccountIDs(ctx, userID)
			if aerr != nil {
				return aerr
			}
			if len(accounts) == 0 {
				break
			}
			id, berr := s.createBudget(ctx, userID, accounts)
			if berr != nil {
				return berr
			}
			out.BudgetId = id
			classifyBudget, classify = vo.MustParseId(id), created
		case writable:
			for _, id := range created {
				if _, aerr := s.steps.Budgets.AddAccount(ctx, userID, model.AddAccountRequest{BudgetId: budgetID.String(), AccountId: id.String()}); aerr != nil {
					return aerr
				}
			}
			if merr := s.syncMembership(ctx, userID, budgetID, hidden, unhidden); merr != nil {
				return merr
			}
			if ferr := s.followMainCurrency(ctx, userID, budgetID, mainID); ferr != nil {
				return ferr
			}
			out.BudgetId = budgetID.String()
			classifyBudget, classify = budgetID, created
		default:
			// A budget the user may only read takes no accounts from them.
			out.BudgetId = budgetID.String()
		}

		done, err := s.finish(ctx, userID, req.Id, StepAccounts)
		if err != nil {
			return err
		}
		out.User = *done
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.scheduleClassify(ctx, userID, classifyBudget, classify)
	return out, nil
}

// reviseAccounts applies the review's currency and balance changes to
// existing accounts. Each must be a live account the user owns and each
// currency one the user may use, else a 400 on the list naming it; a currency
// the account already has is no change.
func (s *Service) reviseAccounts(ctx context.Context, userID vo.Id, currencies []model.OnboardingAccountCurrency, balances []model.OnboardingAccountBalance) error {
	if len(currencies) == 0 && len(balances) == 0 {
		return nil
	}
	own, err := s.steps.Accounts.OwnLiveAccounts(ctx, userID)
	if err != nil {
		return err
	}
	current := make(map[string]string, len(own))
	for _, a := range own {
		current[strings.ToLower(a.ID)] = strings.ToLower(a.CurrencyID)
	}
	byID := map[string]*model.OnboardingAccountRevision{}
	var order []string
	revision := func(id string) *model.OnboardingAccountRevision {
		if r, ok := byID[id]; ok {
			return r
		}
		r := &model.OnboardingAccountRevision{AccountId: id}
		byID[id] = r
		order = append(order, id)
		return r
	}
	for _, c := range currencies {
		id := strings.ToLower(strings.TrimSpace(c.AccountId))
		currencyID := strings.ToLower(strings.TrimSpace(c.CurrencyId))
		was, mine := current[id]
		if !mine {
			return invalidChoice("currencies")
		}
		if was == currencyID {
			continue
		}
		if _, uerr := s.steps.Currencies.UsableCurrencyCode(ctx, userID, currencyID); uerr != nil {
			if refusal(uerr) {
				return invalidChoice("currencies")
			}
			return uerr
		}
		revision(id).CurrencyId = currencyID
	}
	for _, b := range balances {
		id := strings.ToLower(strings.TrimSpace(b.AccountId))
		if _, mine := current[id]; !mine {
			return invalidChoice("balances")
		}
		revision(id).Balance = strings.TrimSpace(b.Balance.String())
	}
	if len(order) == 0 {
		return nil
	}
	apply := make([]model.OnboardingAccountRevision, 0, len(order))
	for _, id := range order {
		apply = append(apply, *byID[id])
	}
	return s.steps.Accounts.ReviseAccounts(ctx, userID, apply)
}

// followMainCurrency puts the budget in the main currency when nothing was
// planned in it yet and nobody else uses it: the budget an earlier step
// (finish-import, save-categories) created took the profile currency of that
// moment, usually the instance base currency.
func (s *Service) followMainCurrency(ctx context.Context, userID, budgetID vo.Id, mainID string) error {
	name, currencyID, ok, err := s.steps.Budgets.UnplannedOwnBudget(ctx, userID, budgetID)
	if err != nil || !ok || strings.EqualFold(currencyID, mainID) {
		return err
	}
	_, err = s.steps.Budgets.UpdateBudget(ctx, userID, model.UpdateBudgetRequest{
		Id: budgetID.String(), Name: name, CurrencyId: mainID,
	})
	return err
}

// visibleFolderID is the user's first visible folder. With none: "" when the
// user has no folder at all and create is false (the first account creates
// one), else a "General" folder, shown again when a hidden one has that name.
func (s *Service) visibleFolderID(ctx context.Context, userID vo.Id, create bool) (string, error) {
	folders, err := s.steps.Accounts.GetFolderList(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, f := range folders.Items {
		if f.IsVisible == 1 {
			return f.Id, nil
		}
	}
	if len(folders.Items) == 0 && !create {
		return "", nil
	}
	for _, f := range folders.Items {
		if f.Name == generalFolderName {
			if _, err := s.steps.Accounts.ShowFolder(ctx, userID, model.ShowFolderRequest{Id: f.Id}); err != nil {
				return "", err
			}
			return f.Id, nil
		}
	}
	res, err := s.steps.Accounts.CreateFolder(ctx, userID, model.CreateFolderRequest{Name: generalFolderName})
	if err != nil {
		return "", err
	}
	return res.Item.Id, nil
}

// generalFolderName is the name the account feature gives the folder it
// creates for a user's first account.
const generalFolderName = "General"

// isArchiveName matches the archive folder's name, plain or numbered.
func isArchiveName(folder, name string) bool {
	if folder == name {
		return true
	}
	n, ok := strings.CutPrefix(folder, name+" ")
	if !ok {
		return false
	}
	_, err := strconv.Atoi(n)
	return err == nil
}

// archiveFolderID is the user's hidden folder named onboarding.archive_folder
// in the request language, created (hidden) when there is none.
func (s *Service) archiveFolderID(ctx context.Context, userID vo.Id) (string, error) {
	name := i18n.T(reqctx.Language(ctx), "onboarding.archive_folder", nil)
	folders, err := s.steps.Accounts.GetFolderList(ctx, userID)
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, f := range folders.Items {
		if f.IsVisible == 0 && isArchiveName(f.Name, name) {
			return f.Id, nil
		}
		taken[f.Name] = true
	}
	// A visible folder of that name is the user's own and is never hidden: the
	// archive gets the first free numbered name instead.
	candidate := name
	for n := 2; taken[candidate]; n++ {
		candidate = name + " " + strconv.Itoa(n)
	}
	res, err := s.steps.Accounts.CreateFolder(ctx, userID, model.CreateFolderRequest{Name: candidate})
	if err != nil {
		return "", err
	}
	if _, err := s.steps.Accounts.HideFolder(ctx, userID, model.HideFolderRequest{Id: res.Item.Id}); err != nil {
		return "", err
	}
	return res.Item.Id, nil
}

// moveAccounts moves the accounts to hide into the archive folder and the
// accounts to unhide into the first visible folder. Each must be a live
// account the user owns (else a 400 on the list naming it); one already where
// it should be does not move. Returns the ids that are hidden and visible now.
func (s *Service) moveAccounts(ctx context.Context, userID vo.Id, own []model.OnboardingAccount, hide, unhide []string) ([]string, []string, error) {
	hiddenNow := map[string]bool{}
	for _, a := range own {
		hiddenNow[strings.ToLower(a.ID)] = a.Hidden
	}
	var hidden, shown []string
	folder := map[bool]string{}
	for _, list := range []struct {
		key  string
		ids  []string
		hide bool
	}{{"hide", hide, true}, {"unhide", unhide, false}} {
		for _, raw := range list.ids {
			id := strings.ToLower(strings.TrimSpace(raw))
			state, mine := hiddenNow[id]
			if !mine {
				return nil, nil, invalidChoice(list.key)
			}
			if list.hide {
				hidden = append(hidden, id)
			} else {
				shown = append(shown, id)
			}
			if state == list.hide {
				continue
			}
			if folder[list.hide] == "" {
				var err error
				if list.hide {
					folder[true], err = s.archiveFolderID(ctx, userID)
				} else {
					folder[false], err = s.visibleFolderID(ctx, userID, true)
				}
				if err != nil {
					return nil, nil, err
				}
			}
			target := folder[list.hide]
			if _, err := s.steps.Accounts.MoveAccount(ctx, userID, model.MoveAccountRequest{Id: id, FolderId: &target}); err != nil {
				return nil, nil, err
			}
			hiddenNow[id] = list.hide
		}
	}
	return hidden, shown, nil
}

// syncMembership takes the hidden accounts out of the budget and puts the
// unhidden ones back. Hiding still succeeds, with the account left a member,
// when remove-account refuses it: history in past periods locks it in the
// budget, or it is a savings member carrying plans or comments. The removal is
// never confirmed, so hiding never deletes a plan or comment the user wrote.
func (s *Service) syncMembership(ctx context.Context, userID, budgetID vo.Id, hidden, shown []string) error {
	for _, id := range hidden {
		_, err := s.steps.Budgets.RemoveAccount(ctx, userID, model.RemoveAccountRequest{BudgetId: budgetID.String(), AccountId: id})
		if err != nil && !hasFieldCode(err, errs.CodeBudgetAccountNotRemovable) && !hasFieldCode(err, errs.CodeBudgetSavingsRemovalUnconfirmed) {
			return err
		}
	}
	for _, id := range shown {
		if _, err := s.steps.Budgets.AddAccount(ctx, userID, model.AddAccountRequest{BudgetId: budgetID.String(), AccountId: id}); err != nil {
			return err
		}
	}
	return nil
}

// visibleCurrencies is the main currency, the requested ones, every new
// account's own and the currency of every live account the user owns (so
// hiding the rest never hides one an account is kept in), deduplicated in that
// order.
func visibleCurrencies(mainID string, req model.OnboardingSaveAccountsRequest, own []model.OnboardingAccount) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	add(mainID)
	for _, id := range req.CurrencyIds {
		add(id)
	}
	for _, a := range req.Accounts {
		add(a.CurrencyId)
	}
	for _, c := range req.Currencies {
		add(c.CurrencyId)
	}
	for _, a := range own {
		add(a.CurrencyID)
	}
	return out
}

// showOnly hides every global currency (the base and profile ones stay) and
// shows the given ones. A currency the user cannot show, or one that does not
// exist, is skipped: each call runs in its own savepoint, so a refusal undoes
// nothing else.
func (s *Service) showOnly(ctx context.Context, userID vo.Id, ids []string) error {
	if _, err := s.steps.Currencies.HideAllCurrencies(ctx, userID); err != nil && !refusal(err) {
		return err
	}
	for _, id := range ids {
		if _, err := s.steps.Currencies.ShowCurrency(ctx, userID, model.ShowCurrencyRequest{Id: id}); err != nil && !refusal(err) {
			return err
		}
	}
	return nil
}

func hasFieldCode(err error, code string) bool {
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, f := range ve.Fields {
		if f.Code == code {
			return true
		}
	}
	return false
}

func refusal(err error) bool {
	var ve *errs.ValidationError
	if errors.As(err, &ve) {
		return true
	}
	_, nf := errs.AsNotFound(err)
	return nf
}

// matchAccounts recovers the key->id map of an earlier call by name among the
// user's live accounts, newest first: the step created its accounts after any
// the user already had, so an older account of the same name is never taken
// for a row. Two rows of one name take two accounts.
func matchAccounts(rows []model.OnboardingAccountRow, own []model.OnboardingAccount) map[string]string {
	own = slices.Clone(own)
	slices.SortStableFunc(own, func(a, b model.OnboardingAccount) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		// UUIDv7 ids order by creation within one second
		return strings.Compare(b.ID, a.ID)
	})
	taken := map[string]bool{}
	out := map[string]string{}
	for _, r := range rows {
		for _, a := range own {
			if !taken[a.ID] && strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(r.Name)) {
				taken[a.ID] = true
				out[r.Key] = a.ID
				break
			}
		}
	}
	return out
}

// SavePlan writes the plan step's amounts as limits of the budget's current
// period (the request's budget, else the user's default one) and marks the
// step done, in one transaction.
func (s *Service) SavePlan(ctx context.Context, userID vo.Id, req model.OnboardingSavePlanRequest) (*model.OnboardingSavePlanResult, error) {
	out := &model.OnboardingSavePlanResult{}
	err := s.steps.Tx.WithTx(ctx, func(ctx context.Context) error {
		user, proceed, err := s.begin(ctx, userID, req.Id, StepPlan)
		if err != nil {
			return err
		}
		out.BudgetId = strings.ToLower(strings.TrimSpace(req.BudgetId))
		if out.BudgetId == "" {
			if out.BudgetId, err = s.defaultBudgetID(ctx, userID, user); err != nil {
				return err
			}
		}
		if !proceed {
			out.User = *user
			return nil
		}
		if out.BudgetId == "" {
			return model.ValidateBlank(map[string]string{"budgetId": ""})
		}
		budgetID := vo.MustParseId(out.BudgetId)
		period, err := s.steps.Budgets.CurrentPeriod(ctx, userID, budgetID)
		if err != nil {
			return err
		}
		for _, a := range req.Amounts {
			amount := vo.NewFlexString(strings.TrimSpace(a.Amount.String()))
			if _, lerr := s.steps.Budgets.SetLimit(ctx, userID, model.SetLimitRequest{
				BudgetId:  out.BudgetId,
				ElementId: strings.TrimSpace(a.CategoryId),
				Period:    period.Format(datetime.DateLayout),
				Amount:    &amount,
			}); lerr != nil {
				return lerr
			}
		}
		done, err := s.finish(ctx, userID, req.Id, StepPlan)
		if err != nil {
			return err
		}
		out.User = *done
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
