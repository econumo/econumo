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

const maxPayeeCandidates = 5

type suggestRow struct {
	Link     model.ImportTransactionLink
	Type     model.TransactionType
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
	active     map[vo.Id]bool
}

type validSuggestion struct {
	Row        suggestRow
	MatchField string
	MatchText  string
	CategoryID *vo.Id
	PayeeID    *vo.Id
}

// suggestReplyRow is step 1's answer for a row; PayeeID is filled from step
// 2, never read from step 1's reply.
type suggestReplyRow struct {
	MatchText  string `json:"matchText"`
	Merchant   string `json:"merchant"`
	CategoryID string `json:"categoryId"`
	PayeeID    string `json:"-"`
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
For each row decide which part of the row's text identifies the merchant, the merchant's common name, and, only when you are highly confident, which category it belongs to.
Rules:
- Include "categoryId" only when highly confident, and only an id from the given list, which holds only categories of the rows' type. Never invent ids.
- "matchText" is the shortest substring of "payee" that identifies the merchant, copied exactly as written.
- "merchant" is the merchant's common name, for example "Amazon" for "AMZN MKTP US*2K4". Leave it out when you cannot tell.
- Names may be in any language; use the timezone and language to read local merchant names.
- Leave out fields and whole rows you are not confident about.
Reply with JSON only: {"rows":{"<key>":{"matchText":"...","merchant":"...","categoryId":"..."}}}`

const payeeSystemPrompt = `You match imported bank and card transactions to payees a user already has in a personal finance app.
Each row lists the transaction's payee text, the merchant's name, and a few candidate payees.
Rules:
- Pick a payee only when it is clearly the same merchant, and only an id from that row's candidates. Never invent ids.
- Leave out rows where no candidate is clearly the same merchant.
Reply with JSON only: {"rows":{"<key>":{"payeeId":"..."}}}`

// buildSuggestPrompt is step 1 for the rows of one type (keys are the rows'
// indexes in rows): the categories of that type, no payees. A row carries only
// the bank's payee text, its type, account name and currency: never an
// amount, a date or the description.
func buildSuggestPrompt(rows []suggestRow, income bool, v suggestVocabulary, language, timezone string) (system, user string) {
	type named struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type promptRow struct {
		Key      string `json:"key"`
		Payee    string `json:"payee"`
		Currency string `json:"currency"`
		Type     string `json:"type"`
		Account  string `json:"account"`
	}
	payload := struct {
		Language   string      `json:"language"`
		Timezone   string      `json:"timezone"`
		Categories []named     `json:"categories"`
		Rows       []promptRow `json:"rows"`
	}{Language: language, Timezone: timezone, Categories: []named{}, Rows: []promptRow{}}
	for _, c := range v.Categories {
		if c.Income == income {
			payload.Categories = append(payload.Categories, named{ID: c.ID, Name: c.Name})
		}
	}
	for i, r := range rows {
		if r.Type.IsIncome() != income {
			continue
		}
		payload.Rows = append(payload.Rows, promptRow{
			Key: strconv.Itoa(i), Payee: r.Link.ExternalPayee, Currency: r.Currency, Type: r.Type.Alias(), Account: r.Account,
		})
	}
	b, _ := json.Marshal(payload)
	return suggestSystemPrompt, string(b)
}

// replyJSON is the reply from its first "{" to its last "}": providers often
// wrap the JSON in a code fence or a sentence.
func replyJSON(reply string) []byte {
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return []byte(reply)
	}
	return []byte(reply[start : end+1])
}

func parseSuggestReply(reply string) (map[string]suggestReplyRow, error) {
	var out struct {
		Rows map[string]suggestReplyRow `json:"rows"`
	}
	if err := json.Unmarshal(replyJSON(reply), &out); err != nil {
		return nil, err
	}
	if out.Rows == nil {
		return nil, errors.New("reply has no rows")
	}
	return out.Rows, nil
}

// buildPayeePrompt is step 2: only the rows with candidates, each with its own.
func buildPayeePrompt(rows []suggestRow, replies map[string]suggestReplyRow, candidates map[string][]model.ImportNamed, language, timezone string) (system, user string) {
	type named struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type promptRow struct {
		Key        string  `json:"key"`
		Payee      string  `json:"payee"`
		Merchant   string  `json:"merchant"`
		Candidates []named `json:"candidates"`
	}
	payload := struct {
		Language string      `json:"language"`
		Timezone string      `json:"timezone"`
		Rows     []promptRow `json:"rows"`
	}{Language: language, Timezone: timezone, Rows: []promptRow{}}
	for i, r := range rows {
		k := strconv.Itoa(i)
		list := candidates[k]
		if len(list) == 0 {
			continue
		}
		pr := promptRow{Key: k, Payee: r.Link.ExternalPayee, Merchant: replies[k].Merchant}
		for _, c := range list {
			pr.Candidates = append(pr.Candidates, named{ID: c.ID, Name: c.Name})
		}
		payload.Rows = append(payload.Rows, pr)
	}
	b, _ := json.Marshal(payload)
	return payeeSystemPrompt, string(b)
}

func parsePayeeReply(reply string) (map[string]string, error) {
	var out struct {
		Rows map[string]struct {
			PayeeID string `json:"payeeId"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(replyJSON(reply), &out); err != nil {
		return nil, err
	}
	if out.Rows == nil {
		return nil, errors.New("reply has no rows")
	}
	picks := map[string]string{}
	for k, r := range out.Rows {
		if r.PayeeID != "" {
			picks[k] = r.PayeeID
		}
	}
	return picks, nil
}

// applyPayeeReply keeps a pick only when it is one of that row's candidates.
func applyPayeeReply(replies map[string]suggestReplyRow, picks map[string]string, candidates map[string][]model.ImportNamed) {
	for k, id := range picks {
		r, ok := replies[k]
		if !ok {
			continue
		}
		for _, c := range candidates[k] {
			if c.ID == id {
				r.PayeeID = id
				replies[k] = r
				break
			}
		}
	}
}

// payeeCandidates ranks the payees by how alike their names are to the
// merchant name or the match text: equal, one containing the other, a small
// edit distance, then a shared word of three letters or more. The AI only
// sees what this keeps, so it never needs the whole payee list.
func payeeCandidates(merchant, matchText string, payees []model.ImportNamed) []model.ImportNamed {
	queries := []string{}
	for _, q := range []string{merchant, matchText} {
		if q = similarityText(q); q != "" {
			queries = append(queries, q)
		}
	}
	type scored struct {
		p     model.ImportNamed
		score float64
	}
	var hits []scored
	for _, p := range payees {
		name := similarityText(p.Name)
		best := 0.0
		for _, q := range queries {
			if sc := nameSimilarity(q, name); sc > best {
				best = sc
			}
		}
		if best > 0 {
			hits = append(hits, scored{p, best})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := []model.ImportNamed{}
	for i := 0; i < len(hits) && i < maxPayeeCandidates; i++ {
		out = append(out, hits[i].p)
	}
	return out
}

func nameSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 4
	}
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	if alnumCount(short) >= 3 && strings.Contains(long, short) {
		return 3
	}
	if r := editRatio(a, b); r >= 0.8 {
		return 2 + r - 0.8
	}
	words := map[string]bool{}
	for _, w := range strings.Fields(a) {
		if letterCount(w) >= 3 {
			words[w] = true
		}
	}
	for _, w := range strings.Fields(b) {
		if words[w] {
			return 1
		}
	}
	return 0
}

// similarityText lowercases, drops apostrophes ("Joe's" reads as "joes") and
// turns every other non-alphanumeric rune into a space.
func similarityText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(NormalizeText(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '\'' || r == '\u2019':
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func letterCount(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// editRatio is 1 - Levenshtein distance / the longer length, in runes. Names
// whose lengths alone rule out 0.8, or longer than 64 runes, score 0 without
// the quadratic work.
func editRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la > lb {
		ra, rb, la, lb = rb, ra, lb, la
	}
	if lb == 0 || lb > 64 || float64(la)/float64(lb) < 0.8 {
		return 0
	}
	prev := make([]int, la+1)
	cur := make([]int, la+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= lb; j++ {
		cur[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[i] = min(prev[i]+1, cur[i-1]+1, prev[i-1]+cost)
		}
		prev, cur = cur, prev
	}
	return 1 - float64(prev[la])/float64(lb)
}

// matchTextIn checks an AI match text against the row's payee text, the only
// row text the AI is ever shown.
func matchTextIn(link model.ImportTransactionLink, raw string) (field, normalized string, ok bool) {
	field, text := model.ImportRuleMatchFieldExternalPayee, link.ExternalPayee
	if strings.TrimSpace(text) == "" {
		return "", "", false
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
