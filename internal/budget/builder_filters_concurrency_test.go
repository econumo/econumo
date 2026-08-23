package budget

// buildFilters runs on the get-budget read path, and ONE process-wide Service
// serves every caller in parallel. A map write there is a
// `fatal error: concurrent map writes` waiting to happen — unrecoverable, so
// the recover middleware cannot turn it into a 500; the process dies. Owners
// must therefore be derived locally from the account views the read already
// loaded. Under -race, two concurrent calls flag any Service-state write.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type accountViewStub struct{ views []model.AccountView }

func (a accountViewStub) AccountsByIDs(context.Context, []vo.Id) ([]model.AccountView, error) {
	return a.views, nil
}

func (a accountViewStub) AccountOwner(context.Context, vo.Id) (vo.Id, error) {
	return vo.Id{}, nil
}

type emptyMetadataStub struct{}

func (emptyMetadataStub) CategoriesByOwners(context.Context, []vo.Id) ([]model.CategoryMeta, error) {
	return nil, nil
}
func (emptyMetadataStub) TagsByOwners(context.Context, []vo.Id) ([]model.TagMeta, error) {
	return nil, nil
}
func (emptyMetadataStub) PayeesByOwners(context.Context, []vo.Id) ([]model.PayeeMeta, error) {
	return nil, nil
}

type frozenClock struct{ t time.Time }

func (c frozenClock) Now() time.Time { return c.t }

func TestBuildFilters_ConcurrentCallsTouchNoServiceState(t *testing.T) {
	ownerID := vo.MustParseId("0000ffff-0000-7000-8000-000000000001")
	currencyID := vo.MustParseId("1a000000-0000-0000-0000-0000000000e1")
	acctA := vo.MustParseId("aaaa0000-0000-7000-8000-000000000001")
	acctB := vo.MustParseId("aaaa0000-0000-7000-8000-000000000002")

	s := &Service{
		read: &spendingStub{},
		accounts: accountViewStub{views: []model.AccountView{
			{ID: acctA.String(), CurrencyID: currencyID.String(), OwnerID: ownerID.String()},
			{ID: acctB.String(), CurrencyID: currencyID.String(), OwnerID: ownerID.String(), IsDeleted: true},
		}},
		metadata:      emptyMetadataStub{},
		clock:         frozenClock{t: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)},
		accountOwners: map[string]string{},
	}
	periodStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	b := &budgetAggregate{
		budget:   &model.Budget{UserID: ownerID, StartedAt: periodStart, CurrencyID: currencyID},
		accounts: []model.BudgetAccount{{AccountID: acctA}, {AccountID: acctB}},
	}

	var wg sync.WaitGroup
	errs := make([]error, 4)
	counts := make([]int, len(errs))
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for n := 0; n < 20; n++ {
				f, err := s.buildFilters(context.Background(), ownerID, b, periodStart, periodStart.AddDate(0, 1, 0))
				if err != nil {
					errs[i] = err
					return
				}
				counts[i] = len(f.accountFilters)
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
		if counts[i] != 2 {
			t.Fatalf("caller %d: accountFilters=%d want 2 (both members, deleted included)", i, counts[i])
		}
	}
}
