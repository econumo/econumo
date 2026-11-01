//go:build upgrade

package upgradetest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/pgsql"
)

// releasedOutOfOrder lists migrations that shipped with a version below one an
// earlier release had already applied. Each is a known hazard with its own
// repair; anything else that sorts below a release's newest migration fails
// the test, because on that release's databases it runs in a different order
// than on a fresh install.
var releasedOutOfOrder = map[string]string{
	// v1.6.0 shipped the access_tokens scope rebuild below v1.5.0's
	// provider/id_token columns and dropped them on v1.5.x databases;
	// 20261031000010-12 repair what it dropped.
	"20260901000000": "access_tokens scope rebuild (v1.6.0, below v1.5.0's 20260907000000)",
	// Order-independent: ADD COLUMNs, and a table no release had stored data
	// in yet. The schema comparison covers them. 20260915000001 was backported
	// to v1.4.2, so v1.4.x databases apply v1.5.0's migrations after it.
	"20260816000001": "accounts.transactions_revision (v1.7.0, below v1.6.x's migrations)",
	"20260907000000": "access_tokens provider/id_token (v1.5.0, below v1.4.2's 20260915000001)",
	"20260908000000": "import_rules recreate (v1.6.0, below v1.5.x's 20260915000001)",
	"20260913000000": "users/oauth_handoffs credentials_generation (v1.5.0, below v1.4.2's 20260915000001)",
}

func TestUpgradeFromRelease(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("releases publish linux binaries only")
	}
	root := repoRoot(t)
	releases, err := sourceReleases(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	current := filepath.Join(t.TempDir(), "econumo")
	if err := buildCurrent(ctx, root, current); err != nil {
		t.Fatal(err)
	}
	t.Logf("upgrading from %s", strings.Join(releases, ", "))

	engines := []engine{sqliteEngine{dir: t.TempDir()}}
	if pg := openPostgres(t); pg != nil {
		engines = append(engines, pg)
	}
	for _, eng := range engines {
		t.Run(eng.name(), func(t *testing.T) {
			fresh := freshInstall(t, eng, current)
			for _, release := range releases {
				t.Run(release, func(t *testing.T) {
					bin, err := releaseBinary(release)
					if errors.Is(err, errNotPublished) {
						t.Skipf("%s: %v", release, err)
					}
					if err != nil {
						t.Fatal(err)
					}
					upgrade(t, eng, release, bin, current, fresh)
				})
			}
		})
	}
}

type installed struct {
	schema   []string
	versions []string
}

// freshInstall is the reference every upgraded database must end up equal to.
func freshInstall(t *testing.T, eng engine, current string) installed {
	t.Helper()
	ctx := context.Background()
	url, err := eng.newDatabase(ctx, "fresh")
	if err != nil {
		t.Fatal(err)
	}
	in := startServer(t, "current (fresh install)", current, url)
	in.stop(t)
	return readInstalled(t, eng, url)
}

func readInstalled(t *testing.T, eng engine, url string) installed {
	t.Helper()
	ctx := context.Background()
	db, err := eng.open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema, err := eng.schema(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := appliedVersions(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return installed{schema: schema, versions: versions}
}

func upgrade(t *testing.T, eng engine, release, oldBin, current string, fresh installed) {
	ctx := context.Background()
	from, ok := parseSemver(release)
	if !ok {
		t.Fatalf("%q is not a vX.Y.Z release", release)
	}
	url, err := eng.newDatabase(ctx, release)
	if err != nil {
		t.Fatal(err)
	}

	// The first boot migrates; currencies beyond the base one only come from
	// the CLI (or rate updates), which needs the migrated schema.
	startServer(t, release, oldBin, url).stop(t)
	runCLI(t, oldBin, url, "currency:add", "EUR", "Euro", "2")
	old := startServer(t, release, oldBin, url)
	s := seed(t, old, from)
	before := takeSnapshot(t, old, s)
	old.stop(t)
	released := readInstalled(t, eng, url).versions

	cur := startServer(t, "current", current, url)
	defer func() {
		if t.Failed() {
			t.Logf("--- %s server log ---\n%s\n--- current server log ---\n%s", release, old.logs(), cur.logs())
		}
	}()
	upgraded := readInstalled(t, eng, url)

	if diff := diffLines(fresh.schema, upgraded.schema); len(diff) > 0 {
		t.Errorf("schema after upgrading from %s differs from a fresh install:\n  %s", release, strings.Join(diff, "\n  "))
	}
	if diff := diffLines(fresh.versions, upgraded.versions); len(diff) > 0 {
		t.Errorf("applied migrations differ from a fresh install:\n  %s", strings.Join(diff, "\n  "))
	}
	checkMigrationOrder(t, release, released, upgraded.versions)
	checkCredentials(t, cur, s)
	checkReads(t, cur, s, before, tolerated(from))
	checkFigures(t, cur, s)
	checkWrites(t, cur, s)

	cur.stop(t)
	again := startServer(t, "current (second boot)", current, url)
	again.client(t, s.ownerSession).get("/api/v1/user/get-user-data", nil)
	again.stop(t)
	for _, in := range []*instance{cur, again} {
		if lines := in.errorLines(); len(lines) > 0 {
			t.Errorf("%s logged errors:\n  %s", in.label, strings.Join(lines, "\n  "))
		}
	}
}

// checkMigrationOrder fails for a migration the release had not applied whose
// version sorts below the release's newest: the runner applies it after
// migrations a fresh install applies after it.
func checkMigrationOrder(t *testing.T, release string, released, upgraded []string) {
	t.Helper()
	if len(released) == 0 {
		t.Fatalf("%s recorded no migrations", release)
	}
	had := map[string]bool{}
	newest := ""
	for _, v := range released {
		had[v] = true
		if v > newest {
			newest = v
		}
	}
	for _, v := range upgraded {
		if had[v] || v > newest {
			continue
		}
		if _, known := releasedOutOfOrder[v]; known {
			continue
		}
		t.Errorf("migration %s is numbered below %s, the newest migration %s applied: a %s database applies it after "+
			"migrations a fresh install applies after it. Renumber it above every released migration.", v, newest, release, release)
	}
}

// checkCredentials: the release's sessions and token keep working, and the
// upgraded database can still mint sessions.
func checkCredentials(t *testing.T, in *instance, s seeded) {
	t.Helper()
	for _, tok := range []struct{ name, token, wantUser string }{
		{"owner session", s.ownerSession, s.ownerID},
		{"partner session", s.partnerSession, s.partnerID},
		{"personal access token", s.pat, s.ownerID},
	} {
		r := in.client(t, tok.token).do("GET", "/api/v1/user/get-user-data", nil)
		var u userEnvelope
		if r.status != 200 || json.Unmarshal(r.data, &u) != nil || u.User.Id != tok.wantUser {
			t.Errorf("%s issued by the release no longer authenticates: HTTP %d %s", tok.name, r.status, r.raw)
		}
	}
	anon := in.client(t, "")
	for _, cred := range [][2]string{{ownerEmail, ownerPassword}, {partnerEmail, partnerPasswd}} {
		login := anon.login(cred[0], cred[1])
		in.client(t, login.Token).get("/api/v1/user/get-user-data", nil)
	}
}

func checkReads(t *testing.T, in *instance, s seeded, before snapshot, ignore func(string) bool) {
	t.Helper()
	after := takeSnapshot(t, in, s)
	for _, r := range reads(s) {
		old, ok := before[r.key()]
		if !ok {
			continue
		}
		cur, ok := after[r.key()]
		if !ok {
			t.Errorf("%s: the release answered, the upgraded binary has no such read", r.key())
			continue
		}
		for _, d := range compareSubset(r.key(), decodeAny(t, old), decodeAny(t, cur), ignore) {
			t.Errorf("%s", d)
		}
	}
}

type accountRow struct {
	Id      string `json:"id"`
	Balance string `json:"balance"`
}

func balances(t *testing.T, c *client) map[string]string {
	t.Helper()
	var list struct {
		Items []accountRow `json:"items"`
	}
	c.get("/api/v1/account/get-account-list", &list)
	out := map[string]string{}
	for _, a := range list.Items {
		out[a.Id] = a.Balance
	}
	return out
}

func wantBalance(t *testing.T, got map[string]string, id, name, want string) {
	t.Helper()
	b, ok := got[id]
	if !ok {
		t.Errorf("%s is missing from the account list", name)
		return
	}
	if !sameScalar(b, want) {
		t.Errorf("%s balance = %s, want %s", name, b, want)
	}
}

// checkFigures pins the numbers the seed implies, independent of what the
// release itself reported.
func checkFigures(t *testing.T, in *instance, s seeded) {
	t.Helper()
	owner := in.client(t, s.ownerSession)
	got := balances(t, owner)
	wantBalance(t, got, s.wallet, "Wallet", walletBalance)
	wantBalance(t, got, s.card, "Euro card", cardBalance)
	if _, ok := got[s.cash]; ok {
		t.Error("the deleted account is listed again")
	}
	wantBalance(t, balances(t, in.client(t, s.partnerSession)), s.wallet, "Wallet shared with the partner", walletBalance)

	var budget struct {
		Item struct {
			Structure struct {
				Elements []struct {
					Id       string `json:"id"`
					Budgeted string `json:"budgeted"`
					Spent    string `json:"spent"`
				} `json:"elements"`
			} `json:"structure"`
		} `json:"item"`
	}
	owner.get("/api/v1/budget/get-budget?id="+s.budget+"&date="+budgetMonthAug, &budget)
	// A tagged transaction counts toward its tag's element, not its category's:
	// the category keeps only the deleted account's 5.00, whose history still
	// counts.
	want := map[string][2]string{s.groceries: {"400", "5"}, s.tag: {"0", "123.45"}}
	for _, e := range budget.Item.Structure.Elements {
		w, ok := want[e.Id]
		if !ok {
			continue
		}
		delete(want, e.Id)
		if !sameScalar(e.Budgeted, w[0]) || !sameScalar(e.Spent, w[1]) {
			t.Errorf("August budget element %s: budgeted %s spent %s, want %s and %s", e.Id, e.Budgeted, e.Spent, w[0], w[1])
		}
	}
	for id := range want {
		t.Errorf("August budget has no element %s", id)
	}

	var txs struct {
		Items []struct {
			Id         string   `json:"id"`
			CategoryId string   `json:"categoryId"`
			PayeeId    string   `json:"payeeId"`
			TagId      string   `json:"tagId"`
			LabelIds   []string `json:"labelIds"`
		} `json:"items"`
	}
	owner.get("/api/v1/transaction/get-transaction-list", &txs)
	for _, tx := range txs.Items {
		if tx.Id != s.expense {
			continue
		}
		if tx.CategoryId != s.groceries || tx.PayeeId != s.payee || tx.TagId != s.tag {
			t.Errorf("groceries expense lost its classification: category %q payee %q tag %q", tx.CategoryId, tx.PayeeId, tx.TagId)
		}
		if s.labels && (len(tx.LabelIds) != 1 || tx.LabelIds[0] != s.label) {
			t.Errorf("groceries expense labels = %v, want [%s]", tx.LabelIds, s.label)
		}
		return
	}
	t.Error("groceries expense is missing from the transaction list")
}

// checkWrites edits rows the release wrote, so a column a migration left in a
// shape the current code cannot write fails here.
func checkWrites(t *testing.T, in *instance, s seeded) {
	t.Helper()
	owner := in.client(t, s.ownerSession)
	owner.post("/api/v1/transaction/create-transaction", map[string]any{
		"id": op(), "type": "expense", "amount": "10.00", "accountId": s.wallet,
		"categoryId": s.groceries, "date": "2026-10-01 12:00:00", "description": "After the upgrade",
	}, nil)
	owner.post("/api/v1/transaction/update-transaction", map[string]any{
		"id": s.expense, "type": "expense", "amount": "100.00", "accountId": s.wallet,
		"categoryId": s.groceries, "payeeId": s.payee, "tagId": s.tag,
		"date": "2026-08-15 10:30:00", "description": "Weekly groceries (corrected)",
	}, nil)
	owner.post("/api/v1/transaction/delete-transaction", map[string]any{"id": s.income}, nil)
	owner.post("/api/v1/budget/set-limit", map[string]any{"budgetId": s.budget, "elementId": s.groceries, "period": budgetMonthSep, "amount": "380"}, nil)
	// 1026.80 - 10 + 23.45 - 250.25
	wantBalance(t, balances(t, owner), s.wallet, "Wallet after post-upgrade writes", "790.00")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// openPostgres returns the PostgreSQL engine, or nil (the pgsql half skips)
// when DATABASE_TEST_PGSQL_URL is unset or unreachable, like the other pgsql
// suites.
func openPostgres(t *testing.T) *pgsqlEngine {
	t.Helper()
	url := os.Getenv("DATABASE_TEST_PGSQL_URL")
	if url == "" {
		t.Log("DATABASE_TEST_PGSQL_URL unset: skipping PostgreSQL")
		return nil
	}
	db, err := pgsql.OpenDB(url)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
	}
	if err != nil {
		// CI provisions PostgreSQL for this job, so there an unreachable one is
		// a broken job, not a reason to test SQLite alone.
		if os.Getenv("CI") != "" {
			t.Fatalf("PostgreSQL at DATABASE_TEST_PGSQL_URL unreachable: %v", err)
		}
		t.Logf("PostgreSQL at DATABASE_TEST_PGSQL_URL unreachable (%v): skipping it", err)
		return nil
	}
	pg := &pgsqlEngine{admin: db, url: url}
	t.Cleanup(func() { pg.cleanup(context.Background()) })
	return pg
}
