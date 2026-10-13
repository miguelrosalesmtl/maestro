package jira

import (
	"encoding/base64"
	"errors"
	"net/http"

	"maestro/internal/secrets"
)

// Authenticator adds credentials to a request. Implementations must not log
// or expose the credential. OAuth will be another implementation;
// callers do not change.
type Authenticator interface {
	Apply(req *http.Request) error
}

// BasicToken is Jira Cloud email + API token.
type BasicToken struct {
	Email string
	Token secrets.Secret
}

func (a BasicToken) Apply(req *http.Request) error {
	if a.Email == "" || a.Token == "" {
		return errors.New("jira: email and API token are required")
	}
	raw := a.Email + ":" + a.Token.Reveal()
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(raw)))
	return nil
}

// BearerToken is a Data Center personal access token.
type BearerToken struct {
	Token secrets.Secret
}

func (a BearerToken) Apply(req *http.Request) error {
	if a.Token == "" {
		return errors.New("jira: personal access token is required")
	}
	req.Header.Set("Authorization", "Bearer "+a.Token.Reveal())
	return nil
}

// Anonymous sends no credentials (setup: tenant_info, serverInfo).
type Anonymous struct{}

func (Anonymous) Apply(*http.Request) error { return nil }
