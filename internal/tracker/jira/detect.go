package jira

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NormalizeSite turns what a user pastes into a site root:
// it adds https://, lower-cases the host, and drops page paths such as
// /browse/PLAT-1 or /jira/software/projects/… . A Data Center context path
// (https://jira.example.com/jira) is kept unless it is followed by a page path.
func NormalizeSite(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return "", errors.New("site is empty")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%q is not a URL", in)
	}
	u.Host = strings.ToLower(u.Host)
	u.RawQuery, u.Fragment, u.User = "", "", nil

	path := u.Path
	if strings.HasSuffix(u.Hostname(), ".atlassian.net") {
		path = "" // Cloud sites have no context path
	} else {
		for _, page := range []string{"/browse/", "/secure/", "/projects/", "/issues/", "/plugins/", "/rest/", "/jira/software/"} {
			if i := strings.Index(path+"/", page); i >= 0 {
				path = path[:i]
			}
		}
	}
	u.Path = strings.TrimRight(path, "/")
	return u.String(), nil
}

// SiteInfo is what setup learns about a site before the user has a token.
type SiteInfo struct {
	BaseURL    string
	Deployment Deployment
	CloudID    string // Cloud only
	Version    string // Data Center only
	Title      string
}

var ErrNotJira = errors.New("this address does not look like a Jira site")

// DetectSite is setup: GET /_edge/tenant_info answers on Jira Cloud;
// otherwise the anonymous /rest/api/2/serverInfo tells Data Center apart.
func DetectSite(ctx context.Context, hc *http.Client, site string) (*SiteInfo, error) {
	base, err := NormalizeSite(site)
	if err != nil {
		return nil, err
	}
	anon, err := New(Options{BaseURL: base, Deployment: Cloud, Auth: Anonymous{}, HTTPClient: hc, Retry: RetryPolicy{MaxServerRetries: 1, MaxRateLimitRetries: 1, BaseDelay: 500 * time.Millisecond, MaxDelay: 2 * time.Second}})
	if err != nil {
		return nil, err
	}

	var tenant struct {
		CloudID string `json:"cloudId"`
	}
	tenantErr := anon.get(ctx, "/_edge/tenant_info", nil, &tenant)
	if tenantErr == nil && tenant.CloudID != "" {
		return &SiteInfo{BaseURL: base, Deployment: Cloud, CloudID: tenant.CloudID}, nil
	}
	if errors.Is(tenantErr, ErrOffline) || ctx.Err() != nil {
		return nil, tenantErr
	}

	var si ServerInfo
	if err := anon.get(ctx, "/rest/api/2/serverInfo", nil, &si); err != nil {
		if errors.Is(err, ErrOffline) {
			return nil, err
		}
		return nil, fmt.Errorf("%w (%s)", ErrNotJira, base)
	}
	switch strings.ToLower(si.DeploymentType) {
	case "cloud":
		return &SiteInfo{BaseURL: base, Deployment: Cloud, Title: si.ServerTitle}, nil
	case "server", "datacenter":
		return &SiteInfo{BaseURL: base, Deployment: DataCenter, Version: si.Version, Title: si.ServerTitle}, nil
	}
	return nil, fmt.Errorf("%w (%s)", ErrNotJira, base)
}
