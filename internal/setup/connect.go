package setup

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"maestro/internal/config"
	"maestro/internal/secrets"
	"maestro/internal/tracker/jira"
)

// Version is sent in the User-Agent.
var Version = "dev"

var ErrNotConfigured = errors.New("jira is not configured; run setup")

// Connect builds a Jira client from the effective configuration. It reads the
// credential from the keychain (or env fallback) and never stores it.
func Connect(cfg *config.Config, store *secrets.Store, log *slog.Logger) (*jira.Client, error) {
	j := cfg.Jira
	if j.Transport != config.TransportREST {
		return nil, fmt.Errorf("jira transport %q has no REST client", j.Transport)
	}
	if j.Site == "" {
		return nil, ErrNotConfigured
	}
	ref, err := secrets.ParseRef(j.Auth.CredentialRef)
	if err != nil {
		return nil, err
	}
	token, err := store.Get(ref)
	if err != nil {
		return nil, fmt.Errorf("jira credential: %w", err)
	}

	dep := jira.Cloud
	if j.Deployment == config.DataCenter {
		dep = jira.DataCenter
	}
	base := j.Site
	var auth jira.Authenticator
	switch j.Auth.Method {
	case config.AuthAPIToken:
		auth = jira.BasicToken{Email: j.Auth.Email, Token: token}
	case config.AuthScopedAPIToken:
		// Scoped tokens go through the API gateway.
		if j.CloudID == "" {
			return nil, errors.New("scoped API tokens need jira.cloudId; run setup to discover it")
		}
		base = "https://api.atlassian.com/ex/jira/" + url.PathEscape(j.CloudID)
		auth = jira.BasicToken{Email: j.Auth.Email, Token: token}
	case config.AuthPAT:
		auth = jira.BearerToken{Token: token}
	default:
		return nil, fmt.Errorf("auth method %q is not supported yet", j.Auth.Method)
	}

	hc, err := jira.NewHTTPClient(jira.NetOptions{
		Timeout: time.Duration(j.HTTP.TimeoutSeconds) * time.Second,
		Proxy:   j.Proxy,
		CAFile:  config.ExpandHome(j.TLS.CAFile),
	})
	if err != nil {
		return nil, err
	}
	return jira.New(jira.Options{
		BaseURL:        base,
		Deployment:     dep,
		Auth:           auth,
		HTTPClient:     hc,
		UserAgent:      "Maestro/" + Version,
		MaxConcurrency: j.HTTP.MaxConcurrency,
		Logger:         log,
	})
}
