// Package archtest enforces the restructure's dependency rule (see
// docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md):
// feature packages never import each other; shared leaves never import
// features; the kernel (internal/shared) imports nothing
// internal outside itself. Features are auto-detected as any internal/<top>
// directory not in the infrastructure set, so newly moved features come under
// enforcement without edits here.
package archtest

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const module = "github.com/econumo/econumo"

// infrastructure lists the internal/<top> dirs that are NOT feature packages.
var infrastructure = map[string]bool{
	"shared": true, "ui": true, "infra": true,
	"server": true, "cli": true, "config": true, "logging": true,
	"test": true,
}

// kernel packages may import internal code only from inside the kernel.
func isKernel(top string) bool { return top == "shared" }

// leaves may be imported by features but must never import one.
// server is deliberately absent: it is the composition root and imports everything.
// config and logging import nothing internal today; listing them keeps that true.
func isLeaf(top string) bool {
	switch top {
	case "shared", "ui", "infra", "config", "logging":
		return true
	}
	return false
}

// topOf extracts the first path segment under internal/ ("" if not internal).
func topOf(pkg string) string {
	rest, ok := strings.CutPrefix(pkg, module+"/internal/")
	if !ok {
		return ""
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

// listImports returns production (non-test) imports for every package in the
// module, via the go tool so build constraints resolve exactly as `go build`.
func listImports(t *testing.T) map[string][]string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	cmd := exec.Command("go", "list", "-f",
		"{{.ImportPath}}|{{range .Imports}}{{.}} {{end}}", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	imports := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, deps, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		imports[pkg] = strings.Fields(deps)
	}
	if len(imports) < 30 {
		t.Fatalf("go list scanned only %d packages — the scan is broken, not the architecture", len(imports))
	}
	return imports
}

func TestDependencyRule(t *testing.T) {
	for pkg, deps := range listImports(t) {
		top := topOf(pkg)
		if top == "" {
			continue // cmd/ and the module root are unconstrained
		}
		feature := !infrastructure[top]
		for _, dep := range deps {
			dtop := topOf(dep)
			if dtop == "" {
				continue
			}
			depFeature := !infrastructure[dtop]
			switch {
			case feature && depFeature && dtop != top:
				t.Errorf("feature %s imports feature %s — features stay decoupled via consumer-side ports wired in internal/server", pkg, dep)
			case isKernel(top) && !isKernel(dtop):
				t.Errorf("kernel %s imports %s — internal/shared imports nothing internal outside the kernel", pkg, dep)
			case !feature && isLeaf(top) && depFeature:
				t.Errorf("leaf %s imports feature %s — shared leaves must not depend on features", pkg, dep)
			}
		}
	}
}
