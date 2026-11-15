// Package prompts embeds the AI system prompts as Markdown files; each file is
// exactly the text the model receives, apart from the {{...}} placeholders
// substituted at init and the trailing newline.
package prompts

import (
	"embed"
	"strings"

	"github.com/econumo/econumo/icons"
)

//go:embed *.md
var files embed.FS

func load(name string) string {
	b, err := files.ReadFile(name)
	if err != nil {
		panic("prompts: " + err.Error())
	}
	return strings.TrimRight(string(b), "\n")
}

var (
	Suggest   = load("suggest.md")
	Payee     = load("payee.md")
	MatchText = load("match_text.md")
	Savings   = load("savings.md")

	iconRules = strings.NewReplacer("{{icons}}", strings.Join(icons.Names, ", ")).Replace(load("icon_rules.md"))

	CategoryIcons = strings.NewReplacer("{{icon_rules}}", iconRules).Replace(load("category_icons.md"))
	AccountIcons  = strings.NewReplacer("{{icon_rules}}", iconRules).Replace(load("account_icons.md"))
)
