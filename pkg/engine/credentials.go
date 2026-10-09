package engine

import (
	"errors"
	"github.com/zalando/go-keyring"
)

var ErrCredentialUnavailable = errors.New("secure credential storage unavailable; choose session-only connection or unlock/configure the OS credential store")

// CredentialStore never persists credentials in the project registry.
type CredentialStore interface {
	Set(string, string) error
	Get(string) (string, error)
	Delete(string) error
}
type SystemCredentials struct{}

const credentialService = "supabase-toys.accounts"

func (SystemCredentials) Set(ref, token string) error {
	if err := keyring.Set(credentialService, ref, token); err != nil {
		return ErrCredentialUnavailable
	}
	return nil
}
func (SystemCredentials) Get(ref string) (string, error) {
	token, err := keyring.Get(credentialService, ref)
	if err != nil {
		return "", ErrCredentialUnavailable
	}
	return token, nil
}
func (SystemCredentials) Delete(ref string) error {
	err := keyring.Delete(credentialService, ref)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return ErrCredentialUnavailable
	}
	return nil
}
