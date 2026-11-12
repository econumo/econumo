package imports

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const maxSuggestPayees = 500

type suggestRow struct {
	Link     model.ImportTransactionLink
	Type     model.TransactionType
	Amount   string
	Currency string
	Account  string
	Missing  struct{ Category, Payee bool }
}

type suggestVocabulary struct {
	Categories []model.ImportNamed
	Payees     []model.ImportNamed
	income     map[vo.Id]bool
}

type validSuggestion struct {
	Row        suggestRow
	MatchField string
	MatchText  string
	CategoryID *vo.Id
	PayeeID    *vo.Id
}

type suggestReplyRow struct {
	MatchText  string `json:"matchText"`
	CategoryID string `json:"categoryId"`
	PayeeID    string `json:"payeeId"`
}

func newSuggestVocabulary(categories, payees []model.ImportNamed) suggestVocabulary {
	if len(payees) > maxSuggestPayees {
		payees = payees[:maxSuggestPayees]
	}
	v := suggestVocabulary{Categories: categories, Payees: payees, income: map[vo.Id]bool{}}
	for _, c := range categories {
		if id, err := vo.ParseId(c.ID); err == nil && c.Income {
			v.income[id] = true
		}
	}
	return v
}

func (v suggestVocabulary) hasCategory(id string) (vo.Id, bool) {
	for _, c := range v.Categories {
		if c.ID == id {
			parsed, err := vo.ParseId(id)
			return parsed, err == nil
		}
	}
	return vo.Id{}, false
}

func (v suggestVocabulary) hasPayee(id string) (vo.Id, bool) {
	for _, p := range v.Payees {
		if p.ID == id {
			parsed, err := vo.ParseId(id)
			return parsed, err == nil
		}
	}
	return vo.Id{}, false
}

const suggestSystemPrompt = `You help classify imported bank and card transactions for a personal finance app.
For each row decide, only when you are highly confident, which category and which payee it belongs to, and which part of the row's text identifies the merchant.
Rules:
- Include "categoryId" or "payeeId" only when highly confident, and only ids from the given lists. Never invent ids.
- A category's "type" must match the row's "type" (income or expense).
- "matchText" is the shortest substring of "payee" (or of "description" when "payee" is empty) that identifies the merchant, copied exactly as written.
- Names may be in any language; use the timezone and language to read local merchant names.
- Leave out fields and whole rows you are not confident about.
Reply with JSON only: {"rows":{"<key>":{"matchText":"...","categoryId":"...","payeeId":"..."}}}`

func buildSuggestPrompt(rows []suggestRow, v suggestVocabulary, language, timezone string) (system, user string) {
	type named struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type,omitempty"`
	}
	type promptRow struct {
		Key         string `json:"key"`
		Payee       string `json:"payee"`
		Description string `json:"description"`
		Amount      string `json:"amount"`
		Currency    string `json:"currency"`
		Type        string `json:"type"`
		Date        string `json:"date"`
		Account     string `json:"account"`
	}
	payload := struct {
		Language   string      `json:"language"`
		Timezone   string      `json:"timezone"`
		Categories []named     `json:"categories"`
		Payees     []named     `json:"payees"`
		Rows       []promptRow `json:"rows"`
	}{Language: language, Timezone: timezone, Categories: []named{}, Payees: []named{}, Rows: []promptRow{}}
	for _, c := range v.Categories {
		t := "expense"
		if c.Income {
			t = "income"
		}
		payload.Categories = append(payload.Categories, named{ID: c.ID, Name: c.Name, Type: t})
	}
	for _, p := range v.Payees {
		payload.Payees = append(payload.Payees, named{ID: p.ID, Name: p.Name})
	}
	for i, r := range rows {
		payload.Rows = append(payload.Rows, promptRow{
			Key: strconv.Itoa(i), Payee: r.Link.ExternalPayee, Description: r.Link.ExternalDescription,
			Amount: r.Amount, Currency: r.Currency, Type: r.Type.Alias(),
			Date: r.Link.ExternalPostedAt.Format("2006-01-02"), Account: r.Account,
		})
	}
	b, _ := json.Marshal(payload)
	return suggestSystemPrompt, string(b)
}

func parseSuggestReply(reply string) (map[string]suggestReplyRow, error) {
	var out struct {
		Rows map[string]suggestReplyRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(reply)), &out); err != nil {
		return nil, err
	}
	if out.Rows == nil {
		return nil, errors.New("reply has no rows")
	}
	return out.Rows, nil
}

func matchTextIn(link model.ImportTransactionLink, raw string) (field, normalized string, ok bool) {
	field, text := model.ImportRuleMatchFieldExternalPayee, link.ExternalPayee
	if strings.TrimSpace(link.ExternalPayee) == "" {
		field, text = model.ImportRuleMatchFieldDescription, link.ExternalDescription
	}
	normalized = NormalizeText(raw)
	if normalized == "" || !strings.Contains(strings.ToLower(NormalizeText(text)), strings.ToLower(normalized)) {
		return "", "", false
	}
	return field, normalized, true
}

func validateReply(rows []suggestRow, reply map[string]suggestReplyRow, v suggestVocabulary) []validSuggestion {
	var out []validSuggestion
	for i, row := range rows {
		r, found := reply[strconv.Itoa(i)]
		if !found {
			continue
		}
		field, text, ok := matchTextIn(row.Link, r.MatchText)
		if !ok {
			continue
		}
		s := validSuggestion{Row: row, MatchField: field, MatchText: text}
		if row.Missing.Category && r.CategoryID != "" {
			if id, ok := v.hasCategory(r.CategoryID); ok && categoryFits(id, v.income, row.Type) {
				s.CategoryID = &id
			}
		}
		if row.Missing.Payee && r.PayeeID != "" {
			if id, ok := v.hasPayee(r.PayeeID); ok {
				s.PayeeID = &id
			}
		}
		if s.CategoryID == nil && s.PayeeID == nil {
			continue
		}
		out = append(out, s)
	}
	return out
}

func idPtrKey(id *vo.Id) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func groupRows(valid []validSuggestion) []model.ImportSuggestion {
	type bucket struct {
		field, text string
		rows        []validSuggestion
	}
	var order []string
	buckets := map[string]*bucket{}
	for _, s := range valid {
		text := strings.ToLower(s.MatchText)
		k := s.MatchField + "\x00" + text
		b, ok := buckets[k]
		if !ok {
			b = &bucket{field: s.MatchField, text: s.MatchText}
			buckets[k] = b
			order = append(order, k)
		}
		b.rows = append(b.rows, s)
	}
	out := make([]model.ImportSuggestion, 0, len(order))
	for _, k := range order {
		b := buckets[k]
		count := map[string]int{}
		for _, s := range b.rows {
			count[idPtrKey(s.CategoryID)+"\x00"+idPtrKey(s.PayeeID)]++
		}
		best := b.rows[0]
		bestN := count[idPtrKey(best.CategoryID)+"\x00"+idPtrKey(best.PayeeID)]
		for _, s := range b.rows[1:] {
			n := count[idPtrKey(s.CategoryID)+"\x00"+idPtrKey(s.PayeeID)]
			if n > bestN || (n == bestN && s.Row.Link.ExternalPostedAt.After(best.Row.Link.ExternalPostedAt)) {
				best, bestN = s, n
			}
		}
		sug := model.ImportSuggestion{
			ID: vo.NewId(), MatchField: b.field, MatchText: b.text,
			CategoryID: best.CategoryID, PayeeID: best.PayeeID, Status: model.ImportSuggestionStatusOpen,
		}
		for _, s := range b.rows {
			sug.LinkIDs = append(sug.LinkIDs, s.Row.Link.ID)
		}
		out = append(out, sug)
	}
	return out
}

func mergeIntoOpen(groups []model.ImportSuggestion, open []model.ImportSuggestion) (fresh []model.ImportSuggestion, joins map[vo.Id][]vo.Id) {
	joins = map[vo.Id][]vo.Id{}
	for _, g := range groups {
		gt := strings.ToLower(g.MatchText)
		joined := false
		for _, o := range open {
			ot := strings.ToLower(o.MatchText)
			if o.MatchField == g.MatchField && (strings.Contains(gt, ot) || strings.Contains(ot, gt)) {
				joins[o.ID] = append(joins[o.ID], g.LinkIDs...)
				joined = true
				break
			}
		}
		if !joined {
			fresh = append(fresh, g)
		}
	}
	return fresh, joins
}

func openFor(row suggestRow, open []model.ImportSuggestion, income map[vo.Id]bool) (vo.Id, bool) {
	ev := model.IngestEvent{Payee: row.Link.ExternalPayee, Description: row.Link.ExternalDescription}
	// Oldest first so the choice is stable when several open suggestions match.
	sorted := append([]model.ImportSuggestion(nil), open...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
	for _, o := range sorted {
		r := model.ImportRule{MatchField: o.MatchField, MatchValue: o.MatchText, MatchType: model.ImportRuleMatchTypeContains}
		if !matchRule(r, ev) {
			continue
		}
		if o.CategoryID != nil && !categoryFits(*o.CategoryID, income, row.Type) {
			continue
		}
		return o.ID, true
	}
	return vo.Id{}, false
}
