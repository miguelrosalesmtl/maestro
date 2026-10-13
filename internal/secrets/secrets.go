// Package secrets keeps credentials out of files.
//
// A credential is addressed by a Ref written in config as
//
//	keychain:<service>/<account>   e.g. keychain:maestro/jira:your-org.atlassian.net:me@example.com
//	env:<VARIABLE>                 e.g. env:MAESTRO_JIRA_TOKEN (read only)
//
// Values are carried as Secret, which never prints itself.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

// Service is the keychain service name Maestro uses.
const Service = "maestro"

// EnvFallback is read when no keychain is available.
const EnvFallback = "MAESTRO_JIRA_TOKEN"

var (
	ErrNotFound   = errors.New("secret not found")
	ErrNoKeychain = errors.New("no OS keychain available")
	ErrReadOnly   = errors.New("secret reference is read only")
	ErrInvalidRef = errors.New("invalid credential reference")
)

// Secret is a credential value. Its String and GoString methods redact it, so
// it is safe to pass through fmt, slog and error wrapping.
type Secret string

func (Secret) String() string               { return "[redacted]" }
func (Secret) GoString() string             { return `secrets.Secret("[redacted]")` }
func (Secret) MarshalText() ([]byte, error) { return []byte("[redacted]"), nil }

// Reveal returns the raw value. Call it only where the value is sent to Jira.
func (s Secret) Reveal() string { return string(s) }

type Scheme string

const (
	SchemeKeychain Scheme = "keychain"
	SchemeEnv      Scheme = "env"
)

type Ref struct {
	Scheme  Scheme
	Service string // keychain only
	Account string // keychain account, or env variable name
}

func (r Ref) String() string {
	if r.Scheme == SchemeEnv {
		return "env:" + r.Account
	}
	return fmt.Sprintf("keychain:%s/%s", r.Service, r.Account)
}

// ParseRef parses a credentialRef value.
func ParseRef(s string) (Ref, error) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || rest == "" {
		return Ref{}, fmt.Errorf("%w %q: want keychain:<service>/<account> or env:<VARIABLE>", ErrInvalidRef, s)
	}
	switch Scheme(scheme) {
	case SchemeKeychain:
		service, account, ok := strings.Cut(rest, "/")
		if !ok || service == "" || account == "" {
			return Ref{}, fmt.Errorf("%w %q: want keychain:<service>/<account>", ErrInvalidRef, s)
		}
		return Ref{Scheme: SchemeKeychain, Service: service, Account: account}, nil
	case SchemeEnv:
		for _, c := range rest {
			if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
				return Ref{}, fmt.Errorf("%w %q: bad environment variable name", ErrInvalidRef, s)
			}
		}
		return Ref{Scheme: SchemeEnv, Account: rest}, nil
	}
	return Ref{}, fmt.Errorf("%w %q: unknown scheme %q", ErrInvalidRef, s, scheme)
}

// JiraRef is the keychain reference for a Jira credential.
func JiraRef(host, user string) Ref {
	return Ref{Scheme: SchemeKeychain, Service: Service, Account: fmt.Sprintf("jira:%s:%s", host, user)}
}

// GitHubRef is the keychain reference for a GitHub token.
func GitHubRef(host string) Ref {
	return Ref{Scheme: SchemeKeychain, Service: Service, Account: "github:" + host}
}

// Store reads and writes secrets. Keychain is the production implementation;
// tests use keyring.MockInit().
type Store struct {
	getenv func(string) string
}

func NewStore() *Store { return &Store{getenv: os.Getenv} }

// Get resolves a reference. If the keychain itself is unavailable, it falls
// back to $MAESTRO_JIRA_TOKEN for Jira references.
func (s *Store) Get(r Ref) (Secret, error) {
	switch r.Scheme {
	case SchemeEnv:
		if v := s.getenv(r.Account); v != "" {
			return Secret(v), nil
		}
		return "", fmt.Errorf("%w: environment variable %s is empty", ErrNotFound, r.Account)
	case SchemeKeychain:
		v, err := keyring.Get(r.Service, r.Account)
		switch {
		case err == nil:
			return Secret(v), nil
		case errors.Is(err, keyring.ErrNotFound):
			return "", fmt.Errorf("%w: %s", ErrNotFound, r)
		default:
			if strings.HasPrefix(r.Account, "jira:") {
				if v := s.getenv(EnvFallback); v != "" {
					return Secret(v), nil
				}
			}
			return "", fmt.Errorf("%w: %v (set %s as an alternative)", ErrNoKeychain, err, EnvFallback)
		}
	}
	return "", ErrInvalidRef
}

// Set stores a secret in the keychain. Env references cannot be written.
func (s *Store) Set(r Ref, v Secret) error {
	if r.Scheme != SchemeKeychain {
		return ErrReadOnly
	}
	if err := keyring.Set(r.Service, r.Account, v.Reveal()); err != nil {
		return fmt.Errorf("%w: %v", ErrNoKeychain, err)
	}
	return nil
}

// Delete removes a keychain secret. Removing a missing secret is not an error.
func (s *Store) Delete(r Ref) error {
	if r.Scheme != SchemeKeychain {
		return ErrReadOnly
	}
	if err := keyring.Delete(r.Service, r.Account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("%w: %v", ErrNoKeychain, err)
	}
	return nil
}
