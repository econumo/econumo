package imports

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"

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
	Missing  missingFields
}

type missingFields struct{ Category, Payee bool }

func (m missingFields) filledBy(o model.ImportSuggestion) bool {
	return (m.Category && o.CategoryID != nil) || (m.Payee && o.PayeeID != nil)
}

type suggestVocabulary struct {
	Categories []model.ImportNamed
	Payees     []model.ImportNamed
	income     map[vo.Id]bool
	// every active category and payee, the payees beyond the prompt's cap too
	active map[vo.Id]bool
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
	v := suggestVocabulary{Categories: categories, income: map[vo.Id]bool{}, active: map[vo.Id]bool{}}
	for _, c := range categories {
		id, err := vo.ParseId(c.ID)
		if err != nil {
			continue
		}
		v.active[id] = true
		if c.Income {
			v.income[id] = true
		}
	}
	for _, p := range payees {
		if id, err := vo.ParseId(p.ID); err == nil {
			v.active[id] = true
		}
	}
	if len(payees) > maxSuggestPayees {
		payees = payees[:maxSuggestPayees]
	}
	v.Payees = payees
	return v
}

// liveOpen drops the fields of open suggestions whose category or payee the
// owner has since archived or deleted, and the suggestions left with none:
// the list read hides such a suggestion, so rows joined to it would be lost.
func (v suggestVocabulary) liveOpen(open []model.ImportSuggestion) []model.ImportSuggestion {
	out := make([]model.ImportSuggestion, 0, len(open))
	for _, o := range open {
		if o.CategoryID != nil && !v.active[*o.CategoryID] {
			o.CategoryID = nil
		}
		if o.PayeeID != nil && !v.active[*o.PayeeID] {
			o.PayeeID = nil
		}
		if o.CategoryID != nil || o.PayeeID != nil {
			out = append(out, o)
		}
	}
	return out
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
	// A text with under three letters or digits ("SQ *") would let openFor's
	// reuse swallow unrelated rows.
	if alnumCount(normalized) < 3 || !strings.Contains(strings.ToLower(NormalizeText(text)), strings.ToLower(normalized)) {
		return "", "", false
	}
	return field, normalized, true
}

func alnumCount(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
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
	groups, _ := groupRowsMissing(valid)
	return groups
}

// groupRowsMissing also returns, parallel to each group's LinkIDs, which
// fields each row lacks, so mergeIntoOpen joins a row only to a suggestion
// that fills one of them.
func groupRowsMissing(valid []validSuggestion) ([]model.ImportSuggestion, [][]missingFields) {
	type bucket struct {
		field, text string
		rows        []validSuggestion
	}
	var order []string
	buckets := map[string]*bucket{}
	for _, s := range valid {
		text := strings.ToLower(s.MatchText)
		k := s.MatchField + "\x00" + text + "\x00" + strconv.FormatBool(s.Row.Type.IsIncome())
		b, ok := buckets[k]
		if !ok {
			b = &bucket{field: s.MatchField, text: s.MatchText}
			buckets[k] = b
			order = append(order, k)
		}
		b.rows = append(b.rows, s)
	}
	var out []model.ImportSuggestion
	var missing [][]missingFields
	for _, k := range order {
		b := buckets[k]
		// The classification of the most rows (ties: the newest row) takes the
		// rows it fills a field of; the others go round again under their own,
		// so no suggestion lists a row it would leave unchanged.
		for rows := b.rows; len(rows) > 0; {
			best := majority(rows)
			sug := model.ImportSuggestion{
				ID: vo.NewId(), MatchField: b.field, MatchText: b.text,
				CategoryID: best.CategoryID, PayeeID: best.PayeeID, Type: rows[0].Row.Type, Status: model.ImportSuggestionStatusOpen,
			}
			var m []missingFields
			var left []validSuggestion
			for _, s := range rows {
				if s.Row.Missing.filledBy(sug) {
					sug.LinkIDs = append(sug.LinkIDs, s.Row.Link.ID)
					m = append(m, s.Row.Missing)
				} else {
					left = append(left, s)
				}
			}
			if len(sug.LinkIDs) == 0 {
				// nothing the remaining rows propose fills them: they cannot be grouped
				break
			}
			out = append(out, sug)
			missing = append(missing, m)
			rows = left
		}
	}
	return out, missing
}

func majority(rows []validSuggestion) validSuggestion {
	count := map[string]int{}
	for _, s := range rows {
		count[idPtrKey(s.CategoryID)+"\x00"+idPtrKey(s.PayeeID)]++
	}
	best := rows[0]
	bestN := count[idPtrKey(best.CategoryID)+"\x00"+idPtrKey(best.PayeeID)]
	for _, s := range rows[1:] {
		n := count[idPtrKey(s.CategoryID)+"\x00"+idPtrKey(s.PayeeID)]
		if n > bestN || (n == bestN && s.Row.Link.ExternalPostedAt.After(best.Row.Link.ExternalPostedAt)) {
			best, bestN = s, n
		}
	}
	return best
}

func sortedOldestFirst(open []model.ImportSuggestion) []model.ImportSuggestion {
	sorted := append([]model.ImportSuggestion(nil), open...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
	return sorted
}

// joinPart is the rows of one group that join one open suggestion.
type joinPart struct {
	group int
	links []vo.Id
}

// groupMissing is parallel to groups (see groupRowsMissing). Each row joins
// the oldest open suggestion of its group's type whose rule ("contains <open
// text>") would match the group and that fills a field the row lacks; rest
// keeps, per group, the rows no open suggestion takes, still under the
// group's own classification.
func mergeIntoOpen(groups []model.ImportSuggestion, groupMissing [][]missingFields, open []model.ImportSuggestion, income map[vo.Id]bool) (rest [][]vo.Id, joins map[vo.Id][]joinPart) {
	joins = map[vo.Id][]joinPart{}
	rest = make([][]vo.Id, len(groups))
	sorted := sortedOldestFirst(open)
	for i, g := range groups {
		gt := strings.ToLower(g.MatchText)
		var candidates []model.ImportSuggestion
		for _, o := range sorted {
			if o.Type != g.Type || o.MatchField != g.MatchField || !strings.Contains(gt, strings.ToLower(o.MatchText)) {
				continue
			}
			if o.CategoryID != nil && !categoryFits(*o.CategoryID, income, g.Type) {
				continue
			}
			candidates = append(candidates, o)
		}
		parts := map[vo.Id]int{}
		for r, link := range g.LinkIDs {
			joined := false
			for _, o := range candidates {
				if !groupMissing[i][r].filledBy(o) {
					continue
				}
				k, ok := parts[o.ID]
				if !ok {
					k = len(joins[o.ID])
					parts[o.ID] = k
					joins[o.ID] = append(joins[o.ID], joinPart{group: i})
				}
				joins[o.ID][k].links = append(joins[o.ID][k].links, link)
				joined = true
				break
			}
			if !joined {
				rest[i] = append(rest[i], link)
			}
		}
	}
	return rest, joins
}

func openFor(row suggestRow, open []model.ImportSuggestion, income map[vo.Id]bool) (vo.Id, bool) {
	ev := model.IngestEvent{Payee: row.Link.ExternalPayee, Description: row.Link.ExternalDescription}
	for _, o := range sortedOldestFirst(open) {
		r := model.ImportRule{MatchField: o.MatchField, MatchValue: o.MatchText, MatchType: model.ImportRuleMatchTypeContains}
		if o.Type != row.Type || !matchRule(r, ev) {
			continue
		}
		if o.CategoryID != nil && !categoryFits(*o.CategoryID, income, row.Type) {
			continue
		}
		if !row.Missing.filledBy(o) {
			continue
		}
		return o.ID, true
	}
	return vo.Id{}, false
}
