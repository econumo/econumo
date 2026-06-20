#!/usr/bin/env bash
# Regenerate names.go from Symfony Intl's en.php currency display-name table.
#
# The Econumo PHP backend resolves a currency's display name via
# Symfony\Component\Intl\Currencies::getName(code) against the `en` locale
# (config default_locale: en), because the stored currencies.name column is NULL
# for every row. To stay byte-compatible, the Go currency module looks the name
# up in the same ICU table. This script parses en.php (element [1] = display
# name) and emits names.go.
#
# Run from the repo root (the dir containing src/ and go/):
#   go/internal/domain/currency/gen_names.sh
#
# Requires python3 and the symfony/intl package present under vendor/ (it ships
# with the PHP backend). No PHP runtime needed.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

SRC=vendor/symfony/intl/Resources/data/currencies/en.php
OUT=go/internal/domain/currency/names.go

python3 - "$SRC" "$OUT" <<'PY'
import re, sys
src = open(sys.argv[1]).read()
pat = re.compile(r"'([A-Z]{3})' => \[\s*\n\s*'((?:[^'\\]|\\.)*)',\s*\n\s*'((?:[^'\\]|\\.)*)',\s*\n\s*\]")
entries = pat.findall(src)
if len(entries) < 150:
    sys.exit("parsed too few currency entries (%d) -- en.php format may have changed" % len(entries))
def goquote(s):
    s = s.replace("\\'", "'").replace('\\\\', '\\')
    return s.replace('\\', '\\\\').replace('"', '\\"')
lines = ['\t"%s": "%s",' % (c, goquote(n)) for c, sym, n in sorted(entries)]
out = '''// Code generated from vendor/symfony/intl/Resources/data/currencies/en.php. DO NOT EDIT.
//
// The Econumo PHP backend resolves a currency's display name via
// Symfony Intl's Currencies::getName(code) against the en locale (config
// default_locale: en), because the stored currencies.name column is NULL for
// every row. To stay byte-compatible, the Go currency module looks the name up
// in this same ICU table (element [1] of each en.php entry) and falls back to
// the code when a code is absent (mirroring the PHP MissingResourceException
// catch). Regenerate with gen_names.sh.

package currency

// currencyNames maps an ISO 4217 code to its English display name.
var currencyNames = map[string]string{
%s
}

// DisplayName returns the English currency name for a code, or the code itself
// when the table has no entry (matching the PHP fallback).
func DisplayName(code string) string {
	if n, ok := currencyNames[code]; ok {
		return n
	}
	return code
}
''' % ("\n".join(lines))
open(sys.argv[2], 'w').write(out)
print("wrote %d currency names to %s" % (len(entries), sys.argv[2]))
PY

gofmt -w "$OUT"
echo "done"
