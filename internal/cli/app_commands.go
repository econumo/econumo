package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	appauthserver "github.com/econumo/econumo/internal/authserver"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	usageAppRegister     = "app:register <name> <redirect-uri>... [--public]"
	usageAppList         = "app:list [--global] [--user]"
	usageAppShow         = "app:show <id>"
	usageAppUpdate       = "app:update <id> [--name <name>] [--redirect-uri <uri>]..."
	usageAppMakeGlobal   = "app:make-global <id>"
	usageAppRotateSecret = "app:rotate-secret <id>"
	usageAppDisconnect   = "app:disconnect <id> [--user <user-id>]"
	usageAppDelete       = "app:delete <id>"
)

// appCommands manage OAuth apps: global ones the operator registers, and the
// user apps MCP clients register for themselves. Secrets are printed once to
// stdout and never logged; audit lines carry ids only.
func appCommands() []command {
	return []command{
		{name: "app:register", summary: "Register a global OAuth app: " + usageAppRegister, run: runAppRegister},
		{name: "app:list", summary: "List OAuth apps: " + usageAppList, run: runAppList},
		{name: "app:show", summary: "Show an OAuth app and its connections: " + usageAppShow, run: runAppShow},
		{name: "app:update", summary: "Rename a global app or replace its redirect URIs: " + usageAppUpdate, run: runAppUpdate},
		{name: "app:make-global", summary: "Turn a user app into a global one: " + usageAppMakeGlobal, run: runAppMakeGlobal},
		{name: "app:rotate-secret", summary: "Replace a confidential app's secret: " + usageAppRotateSecret, run: runAppRotateSecret},
		{name: "app:disconnect", summary: "Revoke an app's connections: " + usageAppDisconnect, run: runAppDisconnect},
		{name: "app:delete", summary: "Revoke an app's connections and delete it: " + usageAppDelete, run: runAppDelete},
	}
}

// parseAppArgs lets flags appear anywhere among the positionals, as "--k v" or
// "--k=v". Bool flags take no value; repeated value flags accumulate. Names
// are given without the leading dashes.
func parseAppArgs(args []string, boolFlags, valueFlags []string) ([]string, map[string][]string, error) {
	var positional []string
	flags := map[string][]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		switch {
		case !strings.HasPrefix(a, "--"):
			return nil, nil, fmt.Errorf("unknown option %q", a)
		case slices.Contains(boolFlags, name):
			if hasValue {
				return nil, nil, fmt.Errorf("option --%s takes no value", name)
			}
			flags[name] = append(flags[name], "")
		case slices.Contains(valueFlags, name):
			if !hasValue {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
					return nil, nil, fmt.Errorf("option --%s needs a value", name)
				}
				i++
				value = args[i]
			}
			flags[name] = append(flags[name], value)
		default:
			return nil, nil, fmt.Errorf("unknown option %q", a)
		}
	}
	return positional, flags, nil
}

func parseAppID(raw string) (vo.Id, error) {
	id, err := vo.ParseId(strings.TrimSpace(raw))
	if err != nil {
		return vo.Id{}, fmt.Errorf("invalid app id %q", raw)
	}
	return id, nil
}

func singleAppID(args []string, usage string) (vo.Id, error) {
	pos, _, err := parseAppArgs(args, nil, nil)
	if err != nil {
		return vo.Id{}, err
	}
	if len(pos) != 1 {
		return vo.Id{}, usageErr(usage)
	}
	return parseAppID(pos[0])
}

func runAppRegister(ctx context.Context, c *container, args []string) error {
	pos, flags, err := parseAppArgs(args, []string{"public"}, nil)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return usageErr(usageAppRegister)
	}
	public := len(flags["public"]) > 0
	client, secret, err := c.apps.RegisterGlobalApp(ctx, pos[0], pos[1:], public)
	if err != nil {
		return err
	}
	slog.Info("app-register", "client_id", client.ID.String(), "public", public)
	fmt.Printf("App %q registered (global).\n", client.Name)
	fmt.Printf("client_id: %s\n", client.ID)
	if public {
		fmt.Println("client_secret: none (public client, PKCE only)")
	} else {
		fmt.Printf("client_secret: %s\n", secret)
		fmt.Println("The secret is shown only once.")
	}
	if !c.apps.Enabled() {
		fmt.Fprintln(os.Stderr, "warning: the OAuth server is disabled (ECONUMO_URL is not an https origin (or an http loopback origin)); the app will work once it is enabled")
	}
	return nil
}

func runAppList(ctx context.Context, c *container, args []string) error {
	pos, flags, err := parseAppArgs(args, []string{"global", "user"}, nil)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usageErr(usageAppList)
	}
	wantGlobal, wantUser := len(flags["global"]) > 0, len(flags["user"]) > 0
	if !wantGlobal && !wantUser {
		wantGlobal, wantUser = true, true
	}
	apps, err := c.apps.ListApps(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tNAME\tREDIRECT\tOWNER\tCREATED\tLAST USED\tCONNECTIONS")
	for _, a := range apps {
		if (a.Client.IsGlobal && !wantGlobal) || (!a.Client.IsGlobal && !wantUser) {
			continue
		}
		redirect := "-"
		if len(a.Client.RedirectURIs) > 0 {
			if host, _ := appauthserver.RedirectHost(a.Client.RedirectURIs[0]); host != "" {
				redirect = host
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", a.Client.ID, appKind(a.Client), a.Client.Name, redirect,
			appOwner(a.Client), formatTime(a.Client.CreatedAt), appLastUsed(a), appConnections(a))
	}
	return w.Flush()
}

func runAppShow(ctx context.Context, c *container, args []string) error {
	id, err := singleAppID(args, usageAppShow)
	if err != nil {
		return err
	}
	a, err := c.apps.GetApp(ctx, id)
	if err != nil {
		return err
	}
	auth := "confidential"
	if a.Client.SecretHash == nil {
		auth = "public"
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintf(w, "ID:\t%s\n", a.Client.ID)
	fmt.Fprintf(w, "Name:\t%s\n", a.Client.Name)
	fmt.Fprintf(w, "Kind:\t%s\n", appKind(a.Client))
	fmt.Fprintf(w, "Owner:\t%s\n", appOwner(a.Client))
	fmt.Fprintf(w, "Auth:\t%s\n", auth)
	fmt.Fprintf(w, "Created:\t%s\n", formatTime(a.Client.CreatedAt))
	fmt.Fprintf(w, "Last used:\t%s\n", appLastUsed(a))
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println("Redirect URIs:")
	for _, u := range a.Client.RedirectURIs {
		fmt.Printf("  %s\n", u)
	}
	fmt.Printf("Connections: %s\n", appConnections(a))
	if len(a.Grants) == 0 {
		return nil
	}
	fmt.Println()
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USER\tSCOPE\tCONNECTED\tLAST USED")
	for _, g := range a.Grants {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", g.UserID, g.Scope, formatTime(g.CreatedAt), formatTime(g.LastUsedAt))
	}
	return w.Flush()
}

func runAppUpdate(ctx context.Context, c *container, args []string) error {
	pos, flags, err := parseAppArgs(args, nil, []string{"name", "redirect-uri"})
	if err != nil {
		return err
	}
	names, uris := flags["name"], flags["redirect-uri"]
	if len(pos) != 1 || len(names) > 1 || (len(names) == 0 && len(uris) == 0) {
		return usageErr(usageAppUpdate)
	}
	id, err := parseAppID(pos[0])
	if err != nil {
		return err
	}
	var name *string
	if len(names) == 1 {
		name = &names[0]
	}
	if err := c.apps.UpdateGlobalApp(ctx, id, name, uris); err != nil {
		return err
	}
	slog.Info("app-update", "client_id", id.String())
	fmt.Printf("App %s updated.\n", id)
	return nil
}

func runAppMakeGlobal(ctx context.Context, c *container, args []string) error {
	id, err := singleAppID(args, usageAppMakeGlobal)
	if err != nil {
		return err
	}
	if err := c.apps.MakeAppGlobal(ctx, id); err != nil {
		return err
	}
	slog.Info("app-make-global", "client_id", id.String())
	fmt.Printf("App %s is now global.\n", id)
	return nil
}

func runAppRotateSecret(ctx context.Context, c *container, args []string) error {
	id, err := singleAppID(args, usageAppRotateSecret)
	if err != nil {
		return err
	}
	secret, err := c.apps.RotateAppSecret(ctx, id)
	if err != nil {
		return err
	}
	slog.Info("app-rotate-secret", "client_id", id.String())
	fmt.Printf("client_secret: %s\n", secret)
	fmt.Println("The old secret no longer works.")
	return nil
}

func runAppDisconnect(ctx context.Context, c *container, args []string) error {
	pos, flags, err := parseAppArgs(args, nil, []string{"user"})
	if err != nil {
		return err
	}
	if len(pos) != 1 || len(flags["user"]) > 1 {
		return usageErr(usageAppDisconnect)
	}
	id, err := parseAppID(pos[0])
	if err != nil {
		return err
	}
	attrs := []any{"client_id", id.String()}
	var userID *vo.Id
	if len(flags["user"]) == 1 {
		raw := flags["user"][0]
		u, err := vo.ParseId(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("invalid user id %q", raw)
		}
		userID = &u
		attrs = append(attrs, "user_id", u.String())
	}
	n, err := c.apps.DisconnectApp(ctx, id, userID)
	if err != nil {
		return err
	}
	slog.Info("app-disconnect", append(attrs, "grants", n)...)
	fmt.Printf("%d connection(s) revoked.\n", n)
	return nil
}

func runAppDelete(ctx context.Context, c *container, args []string) error {
	id, err := singleAppID(args, usageAppDelete)
	if err != nil {
		return err
	}
	n, err := c.apps.DeleteApp(ctx, id)
	if err != nil {
		return err
	}
	slog.Info("app-delete", "client_id", id.String(), "grants", n)
	fmt.Printf("App %s deleted (%d connection(s) revoked).\n", id, n)
	return nil
}

func appKind(c model.OAuthClient) string {
	if c.IsGlobal {
		return "global"
	}
	return "user"
}

func appOwner(c model.OAuthClient) string {
	if c.OwnerUserID == nil {
		return "-"
	}
	return c.OwnerUserID.String()
}

func formatTime(t time.Time) string { return t.UTC().Format(datetime.Layout) }

func appLastUsed(a model.OAuthApp) string {
	var last *time.Time
	if a.Client.LastUsedAt != nil {
		last = a.Client.LastUsedAt
	}
	for i := range a.Grants {
		if last == nil || a.Grants[i].LastUsedAt.After(*last) {
			last = &a.Grants[i].LastUsedAt
		}
	}
	if last == nil {
		return "never"
	}
	return formatTime(*last)
}

// appConnections counts live grants; for a user app it also counts those of
// users other than its owner, which only predate ownership and mark a client
// several people share (a candidate for app:make-global).
func appConnections(a model.OAuthApp) string {
	n := len(a.Grants)
	if a.Client.IsGlobal || a.Client.OwnerUserID == nil {
		return fmt.Sprint(n)
	}
	others := 0
	for _, g := range a.Grants {
		if !g.UserID.Equal(*a.Client.OwnerUserID) {
			others++
		}
	}
	if others == 0 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%d (%d from other users)", n, others)
}
