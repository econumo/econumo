package imports

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Service struct {
	repo      Repository
	accounts  AccountReader
	converter CurrencyConverter
	txns      TransactionCreator
	lister    TransactionLister
	limiter   AttemptLimiter
	tx        port.TxRunner
	clk       port.Clock
	cfg       MatcherConfig
}

func NewService(repo Repository, accounts AccountReader, converter CurrencyConverter, txns TransactionCreator, lister TransactionLister, limiter AttemptLimiter, tx port.TxRunner, clk port.Clock, cfg MatcherConfig) *Service {
	return &Service{repo: repo, accounts: accounts, converter: converter, txns: txns, lister: lister, limiter: limiter, tx: tx, clk: clk, cfg: cfg}
}

// ownedSource resolves a source the caller owns; a foreign id reads as
// not-found so ids cannot be probed.
func (s *Service) ownedSource(ctx context.Context, userID vo.Id, rawID string) (*model.ImportSource, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, errs.NewNotFound("Import source not found")
	}
	src, err := s.repo.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if src.UserID != userID {
		return nil, errs.NewNotFound("Import source not found")
	}
	return src, nil
}

// ownedLink resolves a ledger row through its source's ownership.
func (s *Service) ownedLink(ctx context.Context, userID vo.Id, rawID string) (*model.ImportTransactionLink, *model.ImportSource, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, nil, errs.NewNotFound("Import link not found")
	}
	link, err := s.repo.GetLink(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	src, err := s.ownedSource(ctx, userID, link.SourceID.String())
	if err != nil {
		return nil, nil, errs.NewNotFound("Import link not found")
	}
	return link, src, nil
}

func (s *Service) ownedEvent(ctx context.Context, userID vo.Id, rawID string) (*model.ImportEvent, *model.ImportSource, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, nil, errs.NewNotFound("Import event not found")
	}
	ev, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	src, err := s.ownedSource(ctx, userID, ev.SourceID.String())
	if err != nil {
		return nil, nil, errs.NewNotFound("Import event not found")
	}
	return ev, src, nil
}

// findAccountLink matches a card name case-insensitively: the shortcut
// delivers whatever Wallet renders, and "Apple Card" vs "apple card" is the
// same card.
func findAccountLink(links []model.ImportAccountLink, externalAccountID string) *model.ImportAccountLink {
	for i := range links {
		if strings.EqualFold(links[i].ExternalAccountID, externalAccountID) {
			return &links[i]
		}
	}
	return nil
}

// wallClock drops the offset and keeps the digits: transactions store the
// user's local time as a bare datetime, so a 10:42 -07:00 tap must become a
// 10:42 transaction, and the matcher compares it against spent_at the same way.
func wallClock(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func idString(p *vo.Id) string {
	if p == nil {
		return ""
	}
	return p.String()
}

// sourceResult assembles the wire view of a source: cards are the union of
// the user's explicit links and every card the ledger has seen, so an
// unmapped card shows up with its queued count before the user acts on it.
func (s *Service) sourceResult(ctx context.Context, src *model.ImportSource) (*model.ImportSourceResult, error) {
	accountLinks, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	cards := make([]model.ImportCardResult, 0, len(accountLinks))
	index := map[string]int{}
	for _, al := range accountLinks {
		state := model.ImportCardStateMapped
		if al.Mode == model.ImportAccountLinkModeIgnore {
			state = model.ImportCardStateIgnored
		}
		index[strings.ToLower(al.ExternalAccountID)] = len(cards)
		cards = append(cards, model.ImportCardResult{
			ExternalAccountId: al.ExternalAccountID, ExternalName: al.ExternalName, ExternalCurrency: derefString(al.ExternalCurrency),
			State: state, AccountId: idString(al.AccountID),
		})
	}
	for _, l := range ledger { // newest first (repo order), so the first hit per card is its last-seen tap
		key := strings.ToLower(l.ExternalAccountID)
		i, ok := index[key]
		if !ok {
			index[key] = len(cards)
			cards = append(cards, model.ImportCardResult{ExternalAccountId: l.ExternalAccountID, ExternalName: l.ExternalAccountID, State: model.ImportCardStateUnmapped})
			i = len(cards) - 1
		}
		cards[i].TapCount++
		if l.Status == model.ImportLinkStatusQueued {
			cards[i].QueuedCount++
		}
		if cards[i].LastSeenAt == "" {
			cards[i].LastSeenAt = l.ExternalPostedAt.Format(datetime.Layout)
		}
	}
	return &model.ImportSourceResult{
		Id: src.ID.String(), Provider: src.Provider, Name: src.Name, Status: src.Status,
		CreatedAt: src.CreatedAt.Format(datetime.Layout), Cards: cards,
	}, nil
}
