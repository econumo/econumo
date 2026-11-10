//go:build upgrade

package upgradetest

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// read is one GET a user can make; the release's answer is the baseline the
// upgraded binary must still give.
type read struct {
	who  string // "owner" | "partner"
	path string
}

func reads(s seeded) []read {
	return []read{
		{"owner", "/api/v1/user/get-user-data"},
		{"owner", "/api/v1/account/get-account-list"},
		{"owner", "/api/v1/account/get-folder-list"},
		{"owner", "/api/v1/transaction/get-transaction-list"},
		{"owner", "/api/v1/category/get-category-list"},
		{"owner", "/api/v1/payee/get-payee-list"},
		{"owner", "/api/v1/tag/get-tag-list"},
		{"owner", "/api/v1/label/get-label-list"},
		{"owner", "/api/v1/recurring/get-recurring-transaction-list"},
		{"owner", "/api/v1/budget/get-budget-list"},
		{"owner", "/api/v1/budget/get-budget?id=" + s.budget + "&date=" + budgetMonthAug},
		{"owner", "/api/v1/budget/get-budget?id=" + s.budget + "&date=" + budgetMonthSep},
		{"owner", "/api/v1/connection/get-connection-list"},
		{"owner", "/api/v1/user/get-personal-token-list"},
		{"partner", "/api/v1/account/get-account-list"},
		{"partner", "/api/v1/transaction/get-transaction-list"},
		{"partner", "/api/v1/budget/get-budget-list"},
		{"partner", "/api/v1/budget/get-budget?id=" + s.budget + "&date=" + budgetMonthAug},
		{"partner", "/api/v1/connection/get-connection-list"},
	}
}

type snapshot map[string]json.RawMessage

func (r read) key() string { return r.who + " " + r.path }

// takeSnapshot records every read the release answers; a read it does not
// have yet (404) is left out rather than compared.
func takeSnapshot(t *testing.T, in *instance, s seeded) snapshot {
	t.Helper()
	tokens := map[string]string{"owner": s.ownerSession, "partner": s.partnerSession}
	out := snapshot{}
	for _, r := range reads(s) {
		resp := in.client(t, tokens[r.who]).do("GET", r.path, nil)
		switch resp.status {
		case http.StatusOK:
			out[r.key()] = resp.data
		case http.StatusNotFound:
		default:
			t.Fatalf("%s: GET %s as %s: HTTP %d: %s", in.label, r.path, r.who, resp.status, resp.raw)
		}
	}
	return out
}

// change is a difference an upgrade from a release older than `before` makes
// on purpose. path is a regexp over the paths compareSubset reports
// ("<who> <read> $.items[id=…].field").
type change struct {
	before string
	path   *regexp.Regexp
	why    string
}

var intended = []change{
	{"v1.4.0", regexp.MustCompile(`get-budget\?.* \$\.item\.filters\.excludedAccountsIds$`),
		"explicit budget membership replaced the excluded-accounts list"},
	{"v1.4.0", regexp.MustCompile(`get-budget\?.* \$\.item\.(balances\[\d+\]|structure\.elements\[[^]]+\])\.(spent|budgetSpent|available|expenses|startBalance|endBalance)$`),
		"a budget counts the history of its deleted member accounts"},
	{"v1.4.0", regexp.MustCompile(`get-(folder|account)-list \$\.items\[[^]]+\]\.position$`),
		"fractional sort keys replaced stored positions; position is the index in the new order"},
	{"v1.8.0", regexp.MustCompile(`get-user-data \$\.user\.options\[name=onboarding\]\.value$`),
		"existing users with accounts skip the new onboarding (migration 20261109000000)"},
}

// tolerated returns the path checks a diff from release may ignore.
func tolerated(from semver) func(path string) bool {
	return func(path string) bool {
		for _, c := range intended {
			before, _ := parseSemver(c.before)
			if from.less(before) && c.path.MatchString(path) {
				return true
			}
		}
		return false
	}
}

// compareSubset reports every value of old that cur lost or changed. cur may
// carry extra fields and extra rows (additive wire changes and rows seeded by
// migrations); a row of a list is matched by its id, else its name, else by
// position.
func compareSubset(prefix string, old, cur any, ignore func(string) bool) []string {
	var diffs []string
	var walk func(path string, a, b any)
	walk = func(path string, a, b any) {
		if ignore(path) {
			return
		}
		switch av := a.(type) {
		case map[string]any:
			bv, ok := b.(map[string]any)
			if !ok {
				diffs = append(diffs, fmt.Sprintf("%s: object became %s", path, describe(b)))
				return
			}
			keys := make([]string, 0, len(av))
			for k := range av {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				bval, ok := bv[k]
				if !ok {
					if !ignore(path + "." + k) {
						diffs = append(diffs, fmt.Sprintf("%s.%s: field removed (was %s)", path, k, describe(av[k])))
					}
					continue
				}
				walk(path+"."+k, av[k], bval)
			}
		case []any:
			bv, ok := b.([]any)
			if !ok {
				diffs = append(diffs, fmt.Sprintf("%s: list became %s", path, describe(b)))
				return
			}
			key := rowKey(av)
			if key == "" {
				if len(bv) < len(av) {
					diffs = append(diffs, fmt.Sprintf("%s: %d rows became %d", path, len(av), len(bv)))
					return
				}
				for i := range av {
					walk(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i])
				}
				return
			}
			byKey := map[string]any{}
			for _, row := range bv {
				if m, ok := row.(map[string]any); ok {
					byKey[fmt.Sprint(m[key])] = row
				}
			}
			for _, row := range av {
				id := fmt.Sprint(row.(map[string]any)[key])
				rowPath := fmt.Sprintf("%s[%s=%s]", path, key, id)
				match, ok := byKey[id]
				if !ok {
					diffs = append(diffs, fmt.Sprintf("%s: row missing", rowPath))
					continue
				}
				walk(rowPath, row, match)
			}
		default:
			if !sameScalar(a, b) {
				diffs = append(diffs, fmt.Sprintf("%s: %s became %s", path, describe(a), describe(b)))
			}
		}
	}
	walk(prefix+" $", old, cur)
	return diffs
}

// rowKey returns the field identifying the rows of a list of objects.
func rowKey(rows []any) string {
	for _, key := range []string{"id", "name"} {
		ok := len(rows) > 0
		seen := map[string]bool{}
		for _, row := range rows {
			m, isObj := row.(map[string]any)
			v, has := m[key]
			s := fmt.Sprint(v)
			if !isObj || !has || seen[s] {
				ok = false
				break
			}
			seen[s] = true
		}
		if ok {
			return key
		}
	}
	return ""
}

var decimalText = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// sameScalar treats "1026.8" and "1026.80" as the same amount: the trailing
// zeros a decimal is rendered with are not part of the stored value.
func sameScalar(a, b any) bool {
	if fmt.Sprint(a) == fmt.Sprint(b) && describe(a) == describe(b) {
		return true
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok && decimalText.MatchString(as) && decimalText.MatchString(bs) {
		x, _ := new(big.Rat).SetString(as)
		y, _ := new(big.Rat).SetString(bs)
		return x.Cmp(y) == 0
	}
	return false
}

func describe(v any) string {
	raw, _ := json.Marshal(v)
	s := string(raw)
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return strings.TrimSpace(s)
}

func decodeAny(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}
