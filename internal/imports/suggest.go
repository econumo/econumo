package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	maxSuggestSamples = 200 // classified imports shown to the model
	maxSuggestions    = 50  // rows returned to the client
	maxSuggestReason  = 200 // runes kept from the model's free-text reason
)

const suggestSystemPrompt = `You help a user of a personal finance app write import rules.
The user message is a JSON object with the user's categories, payees, tags and labels (each with id and name)
and "samples": imported transactions (payee, description) paired with the classification the user chose.
Propose rules that would reproduce those choices for future imports.
Reply with ONLY a JSON object of the shape
{"rules":[{"matchField":"external_payee"|"description","matchType":"exact"|"contains"|"prefix",
"matchValue":string,"isCaseSensitive":bool,"categoryId":string|null,"payeeId":string|null,"tagId":string|null,
"labelIds":[string],"reason":string}]}.
Rules:
- Use ONLY ids that appear in the input; never invent ids.
- Prefer "contains" on the shortest stable token of the payee (drop store numbers, cities, card suffixes).
- One rule per distinct merchant; skip samples that contradict each other.
- "reason" is one short sentence for the user, e.g. "4 purchases were filed under Coffee".
- Return at most 30 rules. Return {"rules":[]} when nothing is consistent enough.`

type suggestNamed struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type suggestSample struct {
	Payee       string   `json:"payee"`
	Description string   `json:"description"`
	CategoryId  string   `json:"categoryId,omitempty"`
	PayeeId     string   `json:"payeeId,omitempty"`
	TagId       string   `json:"tagId,omitempty"`
	LabelIds    []string `json:"labelIds,omitempty"`
}

type suggestInput struct {
	Categories []suggestNamed  `json:"categories"`
	Payees     []suggestNamed  `json:"payees"`
	Tags       []suggestNamed  `json:"tags"`
	Labels     []suggestNamed  `json:"labels"`
	Samples    []suggestSample `json:"samples"`
}

type suggestRow struct {
	// Action is read only to DROP a skip row. Re-typing one as classify
	// would show the user a "classify as X" suggestion whose own reason text
	// says to ignore those rows — a rule that does the opposite of what it
	// says. Suggestions are classify-only (spec Part 7 / Part 2).
	Action          string   `json:"action"`
	MatchField      string   `json:"matchField"`
	MatchType       string   `json:"matchType"`
	MatchValue      string   `json:"matchValue"`
	IsCaseSensitive bool     `json:"isCaseSensitive"`
	CategoryId      string   `json:"categoryId"`
	PayeeId         string   `json:"payeeId"`
	TagId           string   `json:"tagId"`
	LabelIds        []string `json:"labelIds"`
	Reason          string   `json:"reason"`
}

type suggestOutput struct {
	Rules []suggestRow `json:"rules"`
}

// SuggestRules asks the configured model for classify rules that reproduce
// the user's past choices on imported rows. Transient by design: nothing is
// stored, and every proposed row is re-validated here — ids the model made
// up, match types it invented, and skip rows are dropped, never surfaced.
func (s *Service) SuggestRules(ctx context.Context, userID vo.Id, req model.SuggestImportRulesRequest) (*model.SuggestImportRulesResult, error) {
	if s.completer == nil {
		return nil, &errs.ValidationError{Msg: "AI suggestions are not enabled on this server", MsgCode: errs.CodeImportAiDisabled}
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeSuggestRules, userID.String()); err != nil {
			return nil, err
		}
	}
	in, err := s.suggestInput(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	res := &model.SuggestImportRulesResult{Items: []model.ImportRuleSuggestion{}}
	if len(in.Samples) == 0 {
		return res, nil // nothing to learn from — and nothing leaves the instance
	}
	if s.limiter != nil {
		// Charged here, not at the gate: the quota exists because a call costs
		// a completion, and the early return above spends none. A completion
		// that then FAILS still counts — it was still bought.
		s.limiter.Fail(RateScopeSuggestRules, userID.String())
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	reply, err := s.completer.Complete(ctx, suggestSystemPrompt, string(payload))
	if err != nil {
		// Type only: the error may quote the endpoint's response body.
		reqctx.AddLogAttr(ctx, "ai_error_type", fmt.Sprintf("%T", err))
		return nil, aiUnavailable()
	}
	rows, ok := parseSuggestions(reply)
	if !ok {
		reqctx.AddLogAttr(ctx, "ai_malformed_reply", true)
		return nil, aiUnavailable()
	}
	own, err := s.ownedEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, row := range rows {
		spec, ok := suggestionSpec(row, own)
		if !ok {
			continue
		}
		key := spec.MatchField + "\x00" + spec.MatchType + "\x00" + strings.ToLower(spec.MatchValue)
		if seen[key] {
			continue
		}
		seen[key] = true
		res.Items = append(res.Items, model.ImportRuleSuggestion{ImportRuleSpec: spec, Reason: clipRunes(strings.TrimSpace(row.Reason), maxSuggestReason)})
		if len(res.Items) == maxSuggestions {
			break
		}
	}
	reqctx.AddLogAttr(ctx, "suggested_count", len(res.Items))
	return res, nil
}

func aiUnavailable() error {
	return &errs.ValidationError{Msg: "The AI service is unavailable. Try again later.", MsgCode: errs.CodeImportAiUnavailable}
}

// suggestInput is the whole payload the model sees: entity names/ids plus
// (payee, description, chosen classification) per imported row that the
// user classified. Amounts, dates, and account names are deliberately absent.
func (s *Service) suggestInput(ctx context.Context, userID vo.Id, req model.SuggestImportRulesRequest) (*suggestInput, error) {
	in := &suggestInput{Categories: []suggestNamed{}, Payees: []suggestNamed{}, Tags: []suggestNamed{}, Labels: []suggestNamed{}, Samples: []suggestSample{}}
	lists := []struct {
		dst  *[]suggestNamed
		list func(context.Context, vo.Id) ([]model.ImportNamed, error)
	}{
		{&in.Categories, s.entities.CategoriesByOwner},
		{&in.Payees, s.entities.PayeesByOwner},
		{&in.Tags, s.entities.TagsByOwner},
		{&in.Labels, s.entities.LabelsByOwner},
	}
	for _, l := range lists {
		rows, err := l.list(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			*l.dst = append(*l.dst, suggestNamed{ID: r.ID, Name: r.Name})
		}
	}
	links, err := s.scopedLinks(ctx, userID, req.Scope, req.RunId, req.ScopeSourceId)
	if err != nil {
		return nil, err
	}
	for i := range links {
		if len(in.Samples) == maxSuggestSamples {
			break
		}
		link := &links[i]
		live, err := s.lister.GetByID(ctx, *link.TransactionID)
		if err != nil {
			if isNotFoundErr(err) {
				continue
			}
			return nil, err
		}
		if live.CategoryID == nil && live.PayeeID == nil && live.TagID == nil && len(live.LabelIDs) == 0 {
			continue // unclassified rows teach the model nothing
		}
		ev := linkEvent(link)
		sample := suggestSample{Payee: ev.Payee, Description: ev.Description}
		if live.CategoryID != nil {
			sample.CategoryId = live.CategoryID.String()
		}
		if live.PayeeID != nil {
			sample.PayeeId = live.PayeeID.String()
		}
		if live.TagID != nil {
			sample.TagId = live.TagID.String()
		}
		for _, id := range live.LabelIDs {
			sample.LabelIds = append(sample.LabelIds, id.String())
		}
		in.Samples = append(in.Samples, sample)
	}
	return in, nil
}

// parseSuggestions tolerates the usual model habits — prose around the
// JSON, a ```json fence — by parsing from the first '{' to the last '}'.
func parseSuggestions(reply string) ([]suggestRow, bool) {
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var out suggestOutput
	if err := json.Unmarshal([]byte(reply[start:end+1]), &out); err != nil {
		return nil, false
	}
	return out.Rules, true
}

// suggestionSpec re-types one model row as a classify rule spec and applies
// the same checks create-rule would: DTO validation plus target ownership.
// Anything the model got wrong drops the row; it never fails the request.
func suggestionSpec(row suggestRow, own ownedIDs) (model.ImportRuleSpec, bool) {
	if a := strings.TrimSpace(row.Action); a != "" && a != model.ImportRuleActionClassify {
		return model.ImportRuleSpec{}, false
	}
	spec := model.ImportRuleSpec{
		Action:          model.ImportRuleActionClassify,
		MatchField:      row.MatchField,
		MatchType:       row.MatchType,
		MatchValue:      strings.TrimSpace(row.MatchValue),
		IsCaseSensitive: row.IsCaseSensitive,
		CategoryId:      optionalID(row.CategoryId),
		PayeeId:         optionalID(row.PayeeId),
		TagId:           optionalID(row.TagId),
		LabelIds:        []string{},
	}
	for _, id := range row.LabelIds {
		if id = strings.TrimSpace(id); id != "" {
			spec.LabelIds = append(spec.LabelIds, id)
		}
	}
	if err := spec.Validate(); err != nil {
		return spec, false
	}
	owned := func(set map[vo.Id]bool, raw *string) bool {
		if raw == nil {
			return true
		}
		id, err := vo.ParseId(*raw)
		return err == nil && set[id]
	}
	if !owned(own.categories, spec.CategoryId) || !owned(own.payees, spec.PayeeId) || !owned(own.tags, spec.TagId) {
		return spec, false
	}
	for _, raw := range spec.LabelIds {
		if !owned(own.labels, &raw) {
			return spec, false
		}
	}
	return spec, true
}

func optionalID(raw string) *string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return &raw
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
