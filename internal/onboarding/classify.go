package onboarding

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const classifySystem = `You help set up a personal budget. A savings account holds money the user is putting aside (a savings or deposit account, an emergency fund, a piggy bank, an investment or brokerage account kept for the future); everyday accounts are for spending (cards, checking, cash, wallets). Account names may be in any language; use the user's timezone and language to read local bank and product names. Decide for each account whether it is a savings account. Reply with JSON only, exactly {"savings":["<id>", ...]} listing the ids of the savings accounts; use an empty list when none is.`

type promptAccount struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

type promptFacts struct {
	Timezone string          `json:"timezone"`
	Language string          `json:"language"`
	Accounts []promptAccount `json:"accounts"`
}

// promptContext captures the request's language and timezone (the explicit
// header, else the stored one) before the request context goes away.
func (s *Service) promptContext(ctx context.Context, userID vo.Id) promptFacts {
	f := promptFacts{Language: reqctx.Language(ctx)}
	if reqctx.IsLocationExplicit(ctx) {
		f.Timezone = reqctx.Location(ctx).String()
	} else if s.timezones != nil {
		f.Timezone, _ = s.timezones.StoredTimezone(ctx, userID)
	}
	return f
}

// classify asks the AI which of the caller's own live accounts are savings and
// flags those in the budget. Accounts of other users and deleted ones are never
// sent. An unusable reply changes nothing.
func (s *Service) classify(ctx context.Context, userID, budgetID vo.Id, accountIDs []vo.Id, facts promptFacts) error {
	accounts, err := s.accounts.AccountsByIDs(ctx, accountIDs)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if a.OwnerID == userID.String() && !a.IsDeleted {
			facts.Accounts = append(facts.Accounts, promptAccount{ID: a.ID, Name: a.Name, Currency: a.CurrencyCode})
		}
	}
	if len(facts.Accounts) == 0 {
		return nil
	}
	body, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	reply, err := s.completer.Complete(ctx, userID.String(), classifySystem, string(body))
	if err != nil {
		return err
	}
	picked, ok := parseReply(reply, facts.Accounts)
	if !ok {
		return MalformedReplyError{}
	}
	if len(picked) == 0 {
		return nil
	}
	return s.budgets.MarkSavingsMembers(ctx, userID, budgetID, picked)
}

// parseReply reads {"savings":[...]} out of the reply (tolerating a code fence
// or prose around it) and keeps only ids that were asked about.
func parseReply(reply string, asked []promptAccount) ([]vo.Id, bool) {
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var parsed struct {
		Savings []string `json:"savings"`
	}
	if err := json.Unmarshal([]byte(reply[start:end+1]), &parsed); err != nil {
		return nil, false
	}
	chosen := map[string]bool{}
	for _, id := range parsed.Savings {
		chosen[strings.ToLower(strings.TrimSpace(id))] = true
	}
	var out []vo.Id
	for _, a := range asked {
		if chosen[strings.ToLower(a.ID)] {
			if id, err := vo.ParseId(a.ID); err == nil {
				out = append(out, id)
			}
		}
	}
	return out, true
}

// MalformedReplyError marks an AI reply that is not the expected JSON; its
// type is all that is logged.
type MalformedReplyError struct{}

func (MalformedReplyError) Error() string { return "malformed AI reply" }
