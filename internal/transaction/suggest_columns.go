package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/prompts"
)

// RateScopeImportColumns caps the column guesses per user: each is a paid AI
// round trip, and the wizard asks once per parsed file.
const RateScopeImportColumns = "import-columns"

// The wizard is already open on the built-in guesses; a late answer is no answer.
const suggestColumnsTimeout = 8 * time.Second

// importColumnRoles is also the precedence when the reply gives one header
// two roles: the earlier role keeps it.
var importColumnRoles = []string{"date", "amount", "amountIn", "amountOut", "description", "payee", "category", "tag", "labels", "account"}

var importColumnsSchema = func() *ai.Schema {
	props := make(map[string]any, len(importColumnRoles))
	for _, role := range importColumnRoles {
		props[role] = map[string]any{"type": []string{"string", "null"}}
	}
	body, err := json.Marshal(map[string]any{
		"type": "object", "properties": props, "required": importColumnRoles, "additionalProperties": false,
	})
	if err != nil {
		panic(err)
	}
	return &ai.Schema{Name: "import_columns", JSON: body}
}()

// SuggestImportColumns asks the AI which CSV header plays which role. Only
// header names, browser-computed column types and the language go out. Every
// way it can come to nothing (no AI, opted out, readonly, over the limit, a
// failed, late or unusable reply) answers empty roles, never an error.
func (s *Service) SuggestImportColumns(ctx context.Context, userID vo.Id, req model.SuggestImportColumnsRequest) (*model.SuggestImportColumnsResult, error) {
	out := &model.SuggestImportColumnsResult{Roles: map[string]string{}}
	if s.columnAI == nil || s.columnFacts == nil {
		reqctx.AddLogAttr(ctx, "outcome", "no_ai")
		return out, nil
	}
	facts, err := s.columnFacts.AIFacts(ctx, userID)
	if err != nil {
		reqctx.AddLogAttr(ctx, "outcome", "failed")
		slog.WarnContext(ctx, "import columns suggestion failed", append([]any{"user_id", userID.String()}, ai.LogAttrs(err)...)...)
		return out, nil
	}
	if facts.AIOff || facts.Readonly {
		reqctx.AddLogAttr(ctx, "outcome", "opted_out")
		return out, nil
	}
	if s.columnLimiter != nil {
		if s.columnLimiter.Allow(ctx, RateScopeImportColumns, userID.String()) != nil || s.columnLimiter.Reserve(RateScopeImportColumns, userID.String()) != nil {
			reqctx.AddLogAttr(ctx, "outcome", "rate_limited")
			return out, nil
		}
	}
	roles, err := s.askColumns(ctx, userID, facts.Language, req.Columns)
	if err != nil {
		reqctx.AddLogAttr(ctx, "outcome", "failed")
		if !errors.Is(err, context.Canceled) {
			// Never log the prompt, the reply or the error text: they can carry header names.
			slog.WarnContext(ctx, "import columns suggestion failed", append([]any{"user_id", userID.String()}, ai.LogAttrs(err)...)...)
		}
		return out, nil
	}
	reqctx.AddLogAttr(ctx, "outcome", "suggested")
	out.Roles = roles
	return out, nil
}

// columnsReplyError marks a reply that is not the expected JSON, so a log
// tells it from a provider failure.
type columnsReplyError struct{}

func (columnsReplyError) Error() string { return "unusable AI reply" }

// askColumns keeps a role only when its value is a header from the request,
// so a model cannot invent a column the wizard does not have.
func (s *Service) askColumns(ctx context.Context, userID vo.Id, language string, columns []model.ImportColumnFact) (map[string]string, error) {
	if language == "" {
		language = "en"
	}
	sent := make([]model.ImportColumnFact, len(columns))
	headers := make(map[string]bool, len(columns))
	for i, c := range columns {
		name := strings.TrimSpace(c.Name)
		sent[i] = model.ImportColumnFact{Name: name, Type: c.Type, Negatives: c.Negatives}
		headers[name] = true
	}
	payload, err := json.Marshal(struct {
		Language string                   `json:"language"`
		Columns  []model.ImportColumnFact `json:"columns"`
	}{language, sent})
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, suggestColumnsTimeout)
	defer cancel()
	reply, err := s.columnAI.Complete(callCtx, userID.String(), prompts.ImportColumns, string(payload), importColumnsSchema)
	if err != nil {
		return nil, err
	}
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, columnsReplyError{}
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(reply[start:end+1]), &parsed); err != nil {
		return nil, columnsReplyError{}
	}
	roles := map[string]string{}
	taken := map[string]bool{}
	for _, role := range importColumnRoles {
		var header string
		if json.Unmarshal(parsed[role], &header) != nil {
			continue
		}
		header = strings.TrimSpace(header)
		// null decodes to "", which would otherwise name a blank header
		if header == "" || !headers[header] || taken[header] {
			continue
		}
		roles[role] = header
		taken[header] = true
	}
	return roles, nil
}
