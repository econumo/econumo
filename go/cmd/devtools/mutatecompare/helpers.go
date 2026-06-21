package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
)

// ---- HTTP client (mirrors apicompare's, plus POST) ----

type client struct {
	base  string
	http  *http.Client
	token string
}

func (c *client) login() error {
	body, _ := json.Marshal(map[string]string{"username": loginEmail, "password": loginPassword})
	req, _ := http.NewRequest(http.MethodPost, c.base+"/api/v1/user/login-user", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var env struct {
		Token string `json:"token"`
		Data  struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode login: %w", err)
	}
	token := env.Token
	if token == "" {
		token = env.Data.Token
	}
	if token == "" {
		return fmt.Errorf("no token: %s", truncate(string(raw), 200))
	}
	c.token = token
	return nil
}

func (c *client) get(path string) (int, []byte, error) {
	req, _ := http.NewRequest(http.MethodGet, c.base+path, nil)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func (c *client) post(path string, body map[string]any) (int, []byte, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

// getData fetches a GET and returns the decoded `data` field.
func (c *client) getData(path string, q url.Values) (any, error) {
	full := path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	status, raw, err := c.get(full)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("status %d", status)
	}
	var env struct {
		Data any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// userID returns the logged-in user's id (data.user.id from get-user-data).
func (c *client) userID() (string, error) {
	data, err := c.getData("/api/v1/user/get-user-data", nil)
	if err != nil {
		return "", err
	}
	if m, ok := data.(map[string]any); ok {
		if u, ok := m["user"].(map[string]any); ok {
			if id, _ := u["id"].(string); id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("no user id in get-user-data")
}

// items returns data.items (or data, if it's an array) as a slice of objects.
func (c *client) items(path string, q url.Values) ([]map[string]any, error) {
	data, err := c.getData(path, q)
	if err != nil {
		return nil, err
	}
	switch t := data.(type) {
	case []any:
		return toObjs(t), nil
	case map[string]any:
		if arr, ok := t["items"].([]any); ok {
			return toObjs(arr), nil
		}
	}
	return nil, nil
}

func toObjs(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// ---- JSON canonicalization (order-insensitive compare) ----

func canonical(raw []byte) ([]byte, error) {
	return canonicalMasked(raw, nil)
}

// canonicalMasked is canonical() with the given JSON field names blanked
// recursively first, so non-deterministic values (fresh ids, now-timestamps)
// don't register as diffs. The fields are set to a fixed sentinel rather than
// removed, so a field PRESENT in one body but absent in the other still differs.
func canonicalMasked(raw []byte, mask []string) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if len(mask) > 0 {
		m := make(map[string]struct{}, len(mask))
		for _, k := range mask {
			m[k] = struct{}{}
		}
		v = maskFields(v, m)
	}
	return marshalCanon(v)
}

func maskFields(v any, mask map[string]struct{}) any {
	switch t := v.(type) {
	case map[string]any:
		for k := range t {
			if _, ok := mask[k]; ok {
				t[k] = "__masked__"
			} else {
				t[k] = maskFields(t[k], mask)
			}
		}
		return t
	case []any:
		for i := range t {
			t[i] = maskFields(t[i], mask)
		}
		return t
	default:
		return v
	}
}

// marshalCanon sorts object keys AND sorts arrays by their canonical element
// string, so ordering-of-ties does not register as a diff (state reads on lists
// have the same tie-ordering quirk as the GET harness documented).
func marshalCanon(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := writeCanon(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeCanon(b *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		b.WriteByte('{')
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			if err := writeCanon(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		// canonicalize each element, then sort by the canonical string.
		elems := make([]string, 0, len(t))
		for _, e := range t {
			eb, err := marshalCanon(e)
			if err != nil {
				return err
			}
			elems = append(elems, string(eb))
		}
		sort.Strings(elems)
		b.WriteByte('[')
		for i, e := range elems {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(e)
		}
		b.WriteByte(']')
	default:
		enc, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(enc)
	}
	return nil
}

// firstDiff returns a short description of the first byte divergence.
func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			start := i - 40
			if start < 0 {
				start = 0
			}
			return fmt.Sprintf("first diff at byte %d:\n          php: ...%s\n          go : ...%s",
				i, snippet(a, start, i+40), snippet(b, start, i+40))
		}
	}
	if len(a) != len(b) {
		return fmt.Sprintf("share prefix but lengths differ (php %dB, go %dB)", len(a), len(b))
	}
	return ""
}

func snippet(b []byte, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(b) {
		end = len(b)
	}
	return string(b[start:end])
}

func printBodies(php, go_ []byte) {
	fmt.Printf("        --- PHP ---\n%s\n        --- GO ---\n%s\n", string(php), string(go_))
}

// classifyListDiff reports "ordering only (N)" when two list responses hold the
// same element multiset in different order, else "content differs (...)" or "".
// Because marshalCanon already sorts arrays, the response/state canonical forms
// would be EQUAL for an ordering-only diff — so a remaining canonical diff is
// genuine content. This still classifies for the human-readable note.
func classifyListDiff(phpBody, goBody []byte) string {
	pl := listElements(phpBody)
	gl := listElements(goBody)
	if pl == nil || gl == nil {
		return ""
	}
	if len(pl) != len(gl) {
		return fmt.Sprintf("content differs (php %d items, go %d items)", len(pl), len(gl))
	}
	pm := multiset(pl)
	gm := multiset(gl)
	missing := 0
	for k, n := range pm {
		if gm[k] != n {
			missing++
		}
	}
	if missing == 0 {
		return fmt.Sprintf("ordering only (%d items)", len(pl))
	}
	return fmt.Sprintf("content differs (%d of %d elements not matched)", missing, len(pl))
}

func listElements(body []byte) []string {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Data) == 0 {
		return nil
	}
	var arr []json.RawMessage
	if json.Unmarshal(env.Data, &arr) != nil {
		var obj struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(env.Data, &obj) != nil || obj.Items == nil {
			return nil
		}
		arr = obj.Items
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		c, err := canonical(e)
		if err != nil {
			return nil
		}
		out = append(out, string(c))
	}
	return out
}

func multiset(xs []string) map[string]int {
	m := make(map[string]int, len(xs))
	for _, x := range xs {
		m[x]++
	}
	return m
}

// newUUID returns a random RFC-4122 v4 UUID string (for create-X ids).
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
