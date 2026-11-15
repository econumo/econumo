// Package icons embeds the icon catalogue (Material Symbols ligature names)
// shared by the SPA (via a Vite JSON import) and the Go binary. The embed
// directive must live in this directory: go:embed cannot reference parent paths.
package icons

import (
	_ "embed"
	"encoding/json"
)

//go:embed icons.json
var raw []byte

var Names = func() []string {
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		panic("icons: malformed icons.json: " + err.Error())
	}
	return names
}()
