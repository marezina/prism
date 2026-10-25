package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/zalando/go-keyring"
)

// runKeyring round-trips a dummy secret through the OS keyring (Windows
// Credential Manager / Linux Secret Service) under a spike-only service name,
// so it never touches a real PRism token.
func runKeyring(args []string) error {
	fs := flag.NewFlagSet("keyring", flag.ExitOnError)
	service := fs.String("service", "PRism-spike", "keyring service name")
	account := fs.String("account", "spike-user", "keyring account name")
	_ = fs.Parse(args)

	const secret = "gho_spike_not_a_real_token_0123456789"

	if err := keyring.Set(*service, *account, secret); err != nil {
		return fmt.Errorf("set: %w", err)
	}
	fmt.Println("set: ok")

	got, err := keyring.Get(*service, *account)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if got != secret {
		return errors.New("get: secret mismatch")
	}
	fmt.Println("get: ok (round-trip matches)")

	if err := keyring.Delete(*service, *account); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	fmt.Println("delete: ok")

	if _, err := keyring.Get(*service, *account); !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("get after delete: want ErrNotFound, got %v", err)
	}
	fmt.Println("get after delete: ErrNotFound as expected")
	return nil
}
