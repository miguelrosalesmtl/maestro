package setup

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"maestro/internal/config"
	"maestro/internal/secrets"
	"maestro/internal/tracker/jira"
)

func TestConnect(t *testing.T) {
	keyring.MockInit()
	store := secrets.NewStore()
	ref := secrets.JiraRef("acme.atlassian.net", "me@acme.com")

	cfg := config.Default()
	cfg.Jira.Site = "https://acme.atlassian.net"
	cfg.Jira.Auth = config.Auth{Method: config.AuthAPIToken, Email: "me@acme.com", CredentialRef: ref.String()}

	if _, err := Connect(cfg, store, nil); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("missing token: %v", err)
	}
	if err := store.Set(ref, "tok"); err != nil {
		t.Fatal(err)
	}
	c, err := Connect(cfg, store, nil)
	if err != nil || c.Deployment() != jira.Cloud || c.BaseURL() != "https://acme.atlassian.net" {
		t.Fatalf("cloud: %v %v", c, err)
	}

	cfg.Jira.Auth.Method = config.AuthScopedAPIToken
	if _, err := Connect(cfg, store, nil); err == nil {
		t.Fatal("scoped token without cloudId must fail")
	}
	cfg.Jira.CloudID = "c-1"
	if c, err := Connect(cfg, store, nil); err != nil || c.BaseURL() != "https://api.atlassian.com/ex/jira/c-1" {
		t.Fatalf("scoped: %v %v", c, err)
	}

	cfg.Jira.Site = ""
	if _, err := Connect(cfg, store, nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no site: %v", err)
	}
}
