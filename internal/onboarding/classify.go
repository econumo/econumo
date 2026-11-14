package onboarding

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type promptAccount struct {
	Key      string `json:"key"`
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
	} else if s.users != nil {
		f.Timezone, _ = s.users.StoredTimezone(ctx, userID)
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
	keys := ai.NewKeys("a")
	for _, a := range accounts {
		if a.OwnerID == userID.String() && !a.IsDeleted {
			facts.Accounts = append(facts.Accounts, promptAccount{Key: keys.Key(a.ID), Name: a.Name, Currency: a.CurrencyCode})
		}
	}
	if len(facts.Accounts) == 0 {
		return nil
	}
	body, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	if !s.mayAsk(ctx, userID) {
		return nil
	}
	reply, err := s.completer.Complete(ctx, userID.String(), classifySystem, string(body), savingsSchema)
	if err != nil {
		return err
	}
	picked, unknown, ok := parseReply(reply, keys)
	if !ok {
		return MalformedReplyError{}
	}
	ai.LogUnknownKeys(userID.String(), reviewSavings.part, unknown)
	if len(picked) == 0 {
		return nil
	}
	return s.budgets.MarkSavingsMembers(ctx, userID, budgetID, picked)
}

// parseReply reads {"savings":[...]} out of the reply (tolerating a code fence
// or prose around it) and maps the keys back to the accounts asked about; it
// also counts the keys the request never issued.
func parseReply(reply string, keys *ai.Keys) ([]vo.Id, int, bool) {
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, 0, false
	}
	var parsed struct {
		Savings []string `json:"savings"`
	}
	if err := json.Unmarshal([]byte(reply[start:end+1]), &parsed); err != nil {
		return nil, 0, false
	}
	var out []vo.Id
	seen := map[string]bool{}
	unknown := 0
	for _, key := range parsed.Savings {
		raw, ok := keys.ID(key)
		if !ok {
			unknown++
			continue
		}
		if id, err := vo.ParseId(raw); err == nil && !seen[raw] {
			seen[raw] = true
			out = append(out, id)
		}
	}
	return out, unknown, true
}

// MalformedReplyError marks an AI reply that is not the expected JSON; its
// type is all that is logged.
type MalformedReplyError struct{}

func (MalformedReplyError) Error() string { return "malformed AI reply" }
