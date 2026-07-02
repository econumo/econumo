#!/usr/bin/env bash
# Regenerate names.go from Symfony Intl's currency data tables.
#
# The Econumo PHP backend resolves currency metadata via Symfony\Component\Intl\
# Currencies against the `en` locale (config default_locale: en):
#   - getName(code)           -> en.php element [1]  (display name)
#   - getSymbol(code)         -> en.php element [0]  (narrow symbol; the code
#                                itself when there is no special symbol)
#   - getFractionDigits(code) -> meta.php 'Meta'[code][0], else 'DEFAULT' (2)
#
# To stay consistent with PHP, the Go currency module looks these up in the same
# ICU tables. This script parses en.php + meta.php and emits names.go (the three
# maps + the DisplayName/Symbol/FractionDigits helpers).
#
# Run from the repo root (the dir containing src/ and go/):
#   go/internal/domain/currency/gen_names.sh
#
# Requires python3 and the symfony/intl package present under vendor/ (it ships
# with the PHP backend). No PHP runtime needed.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

EN=vendor/symfony/intl/Resources/data/currencies/en.php
META=vendor/symfony/intl/Resources/data/currencies/meta.php
OUT=go/internal/domain/currency/names.go

python3 - "$EN" "$META" "$OUT" <<'PY'
import re, sys
en = open(sys.argv[1]).read()
meta = open(sys.argv[2]).read()

# en.php: 'CODE' => [ 'symbol', 'display name', ] -> (code, symbol, name)
en_pat = re.compile(r"'([A-Z]{3})' => \[\s*\n\s*'((?:[^'\\]|\\.)*)',\s*\n\s*'((?:[^'\\]|\\.)*)',\s*\n\s*\]")
entries = en_pat.findall(en)
if len(entries) < 150:
    sys.exit("parsed too few currency entries (%d) -- en.php format may have changed" % len(entries))

# meta.php: restrict to the 'Meta' => [ ... ] block, then 'CODE' => [ <digits>, ...
meta_start = meta.index("'Meta' =>")
meta_block = meta[meta_start:]
frac_pat = re.compile(r"'([A-Z]{3})' => \[\s*\n\s*(\d+),")
fracs = frac_pat.findall(meta_block)
if len(fracs) < 20:
    sys.exit("parsed too few fraction-digit entries (%d) -- meta.php format may have changed" % len(fracs))

def goquote(s):
    s = s.replace("\\'", "'").replace('\\\\', '\\')
    return s.replace('\\', '\\\\').replace('"', '\\"')

name_lines = ['\t"%s": "%s",' % (c, goquote(n)) for c, sym, n in sorted(entries)]
sym_lines  = ['\t"%s": "%s",' % (c, goquote(sym)) for c, sym, n in sorted(entries)]
# Only non-default fraction-digit entries are stored; FractionDigits() falls back
# to the ICU DEFAULT (2) for any code absent from the map.
frac_lines = ['\t"%s": %s,' % (c, d) for c, d in sorted(set(fracs)) if int(d) != 2]

out = '''// Code generated from vendor/symfony/intl/Resources/data/currencies/{en,meta}.php. DO NOT EDIT.
//
// The Econumo PHP backend resolves currency metadata via Symfony Intl's
// Currencies against the en locale (config default_locale: en). To stay
// consistent, the Go currency module looks these up in the same ICU tables:
//   - currencyNames          <- en.php element [1]  (getName)
//   - currencySymbols        <- en.php element [0]  (getSymbol)
//   - currencyFractionDigits <- meta.php Meta[code][0] (getFractionDigits),
//                               with the ICU DEFAULT of 2 for absent codes.
// Each helper falls back to a safe value for an unknown code (the code itself for
// names/symbols, 2 for fraction digits), mirroring the PHP fallbacks.
// Regenerate with gen_names.sh.

package currency

// defaultFractionDigits is the ICU DEFAULT fraction digits (meta.php 'DEFAULT').
const defaultFractionDigits = 2

// currencyNames maps an ISO 4217 code to its English display name.
var currencyNames = map[string]string{
%s
}

// currencySymbols maps an ISO 4217 code to its narrow symbol (the code itself
// when ICU has no special symbol).
var currencySymbols = map[string]string{
%s
}

// currencyFractionDigits maps an ISO 4217 code to its fraction digits when it
// differs from the default (2). Codes absent here use defaultFractionDigits.
var currencyFractionDigits = map[string]int{
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

// Symbol returns the narrow currency symbol for a code, or the code itself when
// the table has no entry (matching PHP Currencies::getSymbol's fallback).
func Symbol(code string) string {
	if s, ok := currencySymbols[code]; ok {
		return s
	}
	return code
}

// FractionDigits returns the ICU fraction digits for a code, defaulting to 2
// (matching PHP Currencies::getFractionDigits' DEFAULT fallback).
func FractionDigits(code string) int {
	if d, ok := currencyFractionDigits[code]; ok {
		return d
	}
	return defaultFractionDigits
}
''' % ("\n".join(name_lines), "\n".join(sym_lines), "\n".join(frac_lines))
open(sys.argv[3], 'w').write(out)
print("wrote %d names, %d symbols, %d non-default fraction-digit entries to %s" % (
    len(entries), len(entries), len(frac_lines), sys.argv[3]))
PY

gofmt -w "$OUT"
echo "done"
