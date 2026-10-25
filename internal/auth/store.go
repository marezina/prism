package auth

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// KeyringService is the OS keyring service name (ARCHITECTURE §6).
const KeyringService = "PRism"

// activeAccount holds the login of the signed-in user, so the token entry
// (account = login) can be found at startup. It is not secret.
const activeAccount = "_active-login"

var (
	ErrNoToken            = errors.New("auth: no stored token")
	ErrKeyringUnavailable = errors.New("auth: OS keyring unavailable (Linux: install and unlock gnome-keyring or KWallet)")
)

// Store keeps the token of the one signed-in account.
type Store interface {
	Save(login, token string) error
	Load() (login, token string, err error)
	Delete() error
}

// KeyringStore stores the token in the OS keyring (ADR 0004).
type KeyringStore struct {
	Service string
}

// NewKeyringStore returns the production store.
func NewKeyringStore() *KeyringStore { return &KeyringStore{Service: KeyringService} }

func (s *KeyringStore) Save(login, token string) error {
	if err := keyring.Set(s.Service, login, token); err != nil {
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	if err := keyring.Set(s.Service, activeAccount, login); err != nil {
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return nil
}

func (s *KeyringStore) Load() (string, string, error) {
	login, err := s.get(activeAccount)
	if err != nil {
		return "", "", err
	}
	token, err := s.get(login)
	if err != nil {
		return "", "", err
	}
	return login, token, nil
}

// Delete removes the token and the active-login pointer. Missing entries are fine.
func (s *KeyringStore) Delete() error {
	login, err := s.get(activeAccount)
	switch {
	case errors.Is(err, ErrNoToken):
		return nil
	case err != nil:
		return err
	}
	for _, account := range []string{login, activeAccount} {
		if err := keyring.Delete(s.Service, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
		}
	}
	return nil
}

func (s *KeyringStore) get(account string) (string, error) {
	v, err := keyring.Get(s.Service, account)
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return "", ErrNoToken
	case err != nil:
		return "", fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return v, nil
}
