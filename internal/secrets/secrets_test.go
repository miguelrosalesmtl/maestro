package secrets

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestParseRef(t *testing.T) {
	good := map[string]Ref{
		"keychain:maestro/jira:acme.atlassian.net:me@x.com": {SchemeKeychain, "maestro", "jira:acme.atlassian.net:me@x.com"},
		"env:MAESTRO_JIRA_TOKEN":                            {SchemeEnv, "", "MAESTRO_JIRA_TOKEN"},
	}
	for in, want := range good {
		got, err := ParseRef(in)
		if err != nil || got != want {
			t.Errorf("ParseRef(%q) = %+v, %v; want %+v", in, got, err, want)
		}
		if got.String() != in {
			t.Errorf("round trip %q → %q", in, got.String())
		}
	}
	for _, bad := range []string{"", "keychain:", "keychain:maestro", "keychain:/x", "env:", "env:BAD-NAME", "file:/tmp/x", "ATATT3xFfGF0token"} {
		if _, err := ParseRef(bad); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ParseRef(%q) err = %v, want ErrInvalidRef", bad, err)
		}
	}
}

func TestSecretNeverPrints(t *testing.T) {
	s := Secret("ATATT3xFfGF0-super-secret")
	var b strings.Builder
	slog.New(slog.NewTextHandler(&b, nil)).Info("auth", "token", s)
	fmt.Fprintf(&b, "%v %s %+v %#v %q", s, s, s, s, s)
	b.WriteString(fmt.Errorf("wrap: %v", s).Error())
	if strings.Contains(b.String(), "super-secret") {
		t.Fatalf("secret leaked: %s", b.String())
	}
	if s.Reveal() != "ATATT3xFfGF0-super-secret" {
		t.Fatal("Reveal must return the raw value")
	}
}

func TestStoreKeychainAndEnv(t *testing.T) {
	keyring.MockInit()
	st := &Store{getenv: func(k string) string { return map[string]string{"MY_TOKEN": "from-env"}[k] }}
	ref := JiraRef("acme.atlassian.net", "me@x.com")

	if _, err := st.Get(ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing secret: err = %v", err)
	}
	if err := st.Set(ref, "tok"); err != nil {
		t.Fatal(err)
	}
	if v, err := st.Get(ref); err != nil || v.Reveal() != "tok" {
		t.Fatalf("Get = %q, %v", v.Reveal(), err)
	}
	if err := st.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(ref); err != nil {
		t.Fatalf("deleting twice must be fine: %v", err)
	}
	if v, err := st.Get(Ref{Scheme: SchemeEnv, Account: "MY_TOKEN"}); err != nil || v.Reveal() != "from-env" {
		t.Fatalf("env Get = %q, %v", v.Reveal(), err)
	}
	if err := st.Set(Ref{Scheme: SchemeEnv, Account: "MY_TOKEN"}, "x"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("env Set err = %v", err)
	}
}

func TestEnvFallbackWhenKeychainMissing(t *testing.T) {
	keyring.MockInitWithError(errors.New("org.freedesktop.secrets was not provided"))
	defer keyring.MockInit()
	st := &Store{getenv: func(k string) string { return map[string]string{EnvFallback: "fallback"}[k] }}
	if v, err := st.Get(JiraRef("h", "u")); err != nil || v.Reveal() != "fallback" {
		t.Fatalf("fallback Get = %q, %v", v.Reveal(), err)
	}
	if _, err := st.Get(GitHubRef("github.com")); !errors.Is(err, ErrNoKeychain) {
		t.Fatalf("non-Jira refs must not use the Jira fallback: %v", err)
	}
}
