package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"prism/internal/auth"
	"prism/internal/gh"
)

func (c *cli) login(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	pat := fs.Bool("pat", false, "read a classic personal access token (ghp_…) from stdin")
	clientID := fs.String("client-id", auth.ClientID, "OAuth App client ID (device flow)")
	_ = fs.Parse(args)

	var (
		id  gh.Identity
		err error
	)
	if *pat {
		id, err = c.loginPAT(ctx)
	} else {
		id, err = c.loginDevice(ctx, *clientID)
	}
	if err != nil {
		return err
	}
	fmt.Printf("Signed in as %s. Token stored in the OS keyring.\n", id.Login)
	return nil
}

func (c *cli) loginPAT(ctx context.Context) (gh.Identity, error) {
	token, err := readSecret("Paste a classic token (scopes: repo, notifications, read:org): ")
	if err != nil {
		return gh.Identity{}, err
	}
	_, id, err := c.session.CompletePAT(ctx, token)
	return id, err
}

func (c *cli) loginDevice(ctx context.Context, clientID string) (gh.Identity, error) {
	flow := auth.NewDeviceFlow(clientID, gh.DotCom, version)
	for {
		dc, err := flow.Start(ctx)
		if err != nil {
			return gh.Identity{}, err
		}
		fmt.Printf("\n  Code: %s\n\n  Enter it at %s (opening your browser).\n  Expires in %s.\n\n",
			dc.UserCode, dc.VerificationURI, time.Until(dc.ExpiresAt).Round(time.Minute))
		if err := openBrowser(dc.VerificationURI); err != nil {
			c.log.Debug("open browser", "err", err)
		}
		fmt.Println("Waiting for authorization…")

		token, err := flow.Wait(ctx, dc)
		switch {
		case errors.Is(err, auth.ErrCodeExpired):
			fmt.Println("The code expired. Requesting a new one…")
			continue
		case errors.Is(err, auth.ErrAccessDenied):
			return gh.Identity{}, errors.New("access was denied in the browser")
		case err != nil:
			return gh.Identity{}, err
		}
		_, id, err := c.session.Complete(ctx, token)
		return id, err
	}
}

// readSecret reads one line without echo when stdin is a terminal.
func readSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read token: %w", err)
		}
		return string(b), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read token from stdin: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
