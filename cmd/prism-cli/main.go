// Command prism-cli is PRism's headless dev tool (ARCHITECTURE §3): sign in,
// inspect the account and, in later phases, watch events and record fixtures.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"prism/internal/auth"
	"prism/internal/gh"
)

// version is set at build time with -ldflags "-X main.version=<tag>".
var version = "dev"

const usage = `usage: prism-cli [-v] <command> [flags]

commands:
  login [--pat] [--client-id ID]   sign in (device flow, or a classic PAT from stdin)
  whoami                           show the signed-in user and token scopes
  orgs                             list organizations (SSO state) and teams
  logout                           forget the stored token
  search "<query>"                 run a GraphQL PR search
  verify-search --owners a,b       check how owner qualifiers and @me behave
  watch [--owners a,b]             poll and print events live (uses the app's database)
  watch --dry [--snapshots]        print the watch set (and snapshot cost)
  unread                           list unread notifications
  read <id>... | all               mark notifications as read
  record --pr o/r#N --out F        write a sanitized fixture snapshot
  sanitize-check [--dir D]         fail if a fixture contains original names
`

func main() {
	verbose := flag.Bool("v", false, "debug logging (requests, rate limits)")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app := &cli{log: log}
	app.session = &auth.Session{
		Store:     auth.NewKeyringStore(),
		NewClient: app.newClient,
	}

	args := flag.Args()[1:]
	var err error
	switch flag.Arg(0) {
	case "login":
		err = app.login(ctx, args)
	case "whoami":
		err = app.whoami(ctx)
	case "orgs":
		err = app.orgs(ctx)
	case "logout":
		err = app.session.Logout()
		if err == nil {
			fmt.Println("Signed out. The token was removed from the keyring.")
		}
	case "search":
		err = app.search(ctx, args)
	case "verify-search":
		err = app.verifySearch(ctx, args)
	case "watch":
		err = app.watch(ctx, args)
	case "unread":
		err = app.unread(ctx)
	case "read":
		err = app.read(ctx, args)
	case "record":
		err = app.record(ctx, args)
	case "sanitize-check":
		err = app.sanitizeCheck(args)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if errors.Is(err, auth.ErrNoToken) {
			fmt.Fprintln(os.Stderr, "Run `prism-cli login` first.")
		}
		os.Exit(1)
	}
}

type cli struct {
	log     *slog.Logger
	session *auth.Session
}

func (c *cli) newClient(token string) (*gh.Client, error) {
	return gh.New(gh.Options{Token: token, Version: version, Logger: c.log})
}

// client restores the stored session.
func (c *cli) client(ctx context.Context) (*gh.Client, gh.Identity, error) {
	client, id, err := c.session.Restore(ctx)
	if errors.Is(err, gh.ErrUnauthorized) {
		return nil, gh.Identity{}, fmt.Errorf("%w; the stored token was removed, sign in again", err)
	}
	return client, id, err
}
