// Command mutatecompare diffs the behavior of WRITE (POST) endpoints between the
// PHP and Go Econumo backends. Unlike apicompare (read-only GETs), each mutation
// permanently changes the database, so correctness depends on BOTH backends
// starting from a byte-identical database state for the case.
//
// The driver script (deployment/compare/mutate_compare.sh) provides that
// isolation: per case it stops both backends, copies a fresh seed DB for each,
// restarts them, then runs THIS tool for exactly one case (-case <name>). This
// tool then:
//
//  1. Derives inputs from a seed READ (e.g. picks a real category id to update).
//     The same derivation runs against PHP; the resulting body is sent to BOTH so
//     the two mutations are identical.
//  2. POSTs the identical body to both backends and compares the response
//     envelope (status + canonicalized JSON, order-insensitive).
//  3. Performs a follow-up READ on both and compares the resulting STATE — the
//     real effect of the mutation (the response alone can hide a persistence bug).
//
// It prints a PASS/DIFF verdict and exits non-zero on any diff. It never prints
// field values unless -v is set (compliance: the seed may hold personal data).
//
// Usage:
//
//	go run ./cmd/devtools/mutatecompare -case category/create -php http://localhost:8082 -go http://localhost:8282
//	go run ./cmd/devtools/mutatecompare -list   # list all case names
package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	loginEmail    = "" // supply via -email (no creds committed)
	loginPassword = "" // supply via -password
)

func main() {
	phpBase := flag.String("php", "http://localhost:8082", "base URL of the PHP backend")
	goBase := flag.String("go", "http://localhost:8282", "base URL of the Go backend")
	caseName := flag.String("case", "", "the single mutation case to run (see -list)")
	list := flag.Bool("list", false, "list all case names and exit")
	verbose := flag.Bool("v", false, "print bodies on mismatch (may contain personal data)")
	email := flag.String("email", loginEmail, "login email")
	password := flag.String("password", loginPassword, "login password")
	flag.Parse()
	loginEmail, loginPassword = *email, *password

	cases := allCases()
	if *list {
		for _, c := range cases {
			fmt.Println(c.name)
		}
		return
	}
	if *caseName == "" {
		fmt.Fprintln(os.Stderr, "must pass -case <name> (or -list)")
		os.Exit(2)
	}
	var tc *mutationCase
	for i := range cases {
		if cases[i].name == *caseName {
			tc = &cases[i]
			break
		}
	}
	if tc == nil {
		fmt.Fprintf(os.Stderr, "unknown case %q (see -list)\n", *caseName)
		os.Exit(2)
	}

	php := &client{base: strings.TrimRight(*phpBase, "/"), http: &http.Client{Timeout: 30 * time.Second}}
	go_ := &client{base: strings.TrimRight(*goBase, "/"), http: &http.Client{Timeout: 30 * time.Second}}
	if err := php.login(); err != nil {
		fmt.Fprintf(os.Stderr, "PHP login failed: %v\n", err)
		os.Exit(1)
	}
	if err := go_.login(); err != nil {
		fmt.Fprintf(os.Stderr, "Go login failed: %v\n", err)
		os.Exit(1)
	}

	if runCase(php, go_, tc, *verbose) {
		os.Exit(0)
	}
	os.Exit(1)
}

// mutationCase describes one POST endpoint comparison.
type mutationCase struct {
	name string
	// build derives the POST body from a seed read (run against php; the same
	// body is sent to both backends). Returns the path + body, or skip=true with
	// a reason when the seed lacks the data this case needs.
	build func(php *client) (path string, body map[string]any, skip string, err error)
	// stateRead is the GET path (with query) used to diff resulting state. Empty
	// means response-only comparison.
	stateRead func(php *client) string
	// volatile lists JSON field names whose values are non-deterministic between
	// backends (a freshly-minted UUIDv7 entity id, or a created/updated timestamp
	// set to "now") and so must be blanked before comparing. Applied recursively
	// to BOTH the response and the state bodies. Use for create-* cases.
	volatile []string
}

func runCase(php, go_ *client, tc *mutationCase, verbose bool) bool {
	path, body, skip, err := tc.build(php)
	if err != nil {
		fmt.Printf("[ERR ] %-42s build: %v\n", tc.name, err)
		return false
	}
	if skip != "" {
		fmt.Printf("[SKIP] %-42s %s\n", tc.name, skip)
		return true
	}

	pStatus, pBody, pErr := php.post(path, body)
	gStatus, gBody, gErr := go_.post(path, body)
	if pErr != nil || gErr != nil {
		fmt.Printf("[ERR ] %-42s php=%v go=%v\n", tc.name, pErr, gErr)
		return false
	}

	ok := true
	// 1. Response envelope.
	if pStatus != gStatus {
		fmt.Printf("[DIFF] %-42s response status php=%d go=%d\n", tc.name, pStatus, gStatus)
		ok = false
	}
	pc, pe := canonicalMasked(pBody, tc.volatile)
	gc, ge := canonicalMasked(gBody, tc.volatile)
	if pe == nil && ge == nil && !bytes.Equal(pc, gc) {
		kind := classifyListDiff(pBody, gBody)
		fmt.Printf("[DIFF] %-42s response body differs (%d/%d) %s\n", tc.name, pStatus, gStatus, kind)
		if d := firstDiff(pc, gc); d != "" {
			fmt.Printf("        %s\n", d)
		}
		if verbose {
			printBodies(pc, gc)
		}
		ok = false
	}

	// 2. Resulting state.
	if tc.stateRead != nil {
		sp := tc.stateRead(php)
		_, psBody, pse := php.get(sp)
		_, gsBody, gse := go_.get(sp)
		if pse != nil || gse != nil {
			fmt.Printf("[ERR ] %-42s state read php=%v go=%v\n", tc.name, pse, gse)
			return false
		}
		psc, _ := canonicalMasked(psBody, tc.volatile)
		gsc, _ := canonicalMasked(gsBody, tc.volatile)
		if !bytes.Equal(psc, gsc) {
			kind := classifyListDiff(psBody, gsBody)
			if kind == "" || strings.HasPrefix(kind, "content") {
				fmt.Printf("[DIFF] %-42s STATE differs after mutation %s\n", tc.name, kind)
				if d := firstDiff(psc, gsc); d != "" {
					fmt.Printf("        %s\n", d)
				}
				if verbose {
					printBodies(psc, gsc)
				}
				ok = false
			} else {
				// ordering-only state diff is accepted (matches GET classification).
				fmt.Printf("[note] %-42s state ordering-only (%s) — accepted\n", tc.name, kind)
			}
		}
	}

	if ok {
		fmt.Printf("[PASS] %-42s (resp %d, state checked)\n", tc.name, pStatus)
	}
	return ok
}
