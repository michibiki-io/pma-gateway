package auth

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/michibiki-io/pma-gateway/backend/internal/config"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cidr, err := parseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	return config.Config{
		UserHeader:        "Remote-User",
		GroupsHeader:      "Remote-Groups",
		GroupsSeparator:   ",",
		AdminUsers:        map[string]struct{}{"admin@example.com": {}},
		AdminGroups:       map[string]struct{}{"db-admins": {}},
		TrustedProxyCIDRs: cidr,
		TrustProxyHeaders: true,
	}
}

func TestFromRequestExtractsTrustedHeaderIdentity(t *testing.T) {
	cfg := testConfig(t)
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.RemoteAddr = "10.1.2.3:4321"
	req.Header.Set("Remote-User", "alice@example.com")
	req.Header.Set("Remote-Groups", "db-users, db-admins")
	req.Header.Set("X-Forwarded-For", "198.51.100.10")
	identity, err := FromRequest(req, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if identity.User != "alice@example.com" || !identity.IsAdmin {
		t.Fatalf("unexpected identity: %+v", identity)
	}
	if identity.RemoteAddress != "198.51.100.10" {
		t.Fatalf("remote address = %q", identity.RemoteAddress)
	}
}

func TestFromRequestRejectsUntrustedProxy(t *testing.T) {
	cfg := testConfig(t)
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.RemoteAddr = "203.0.113.20:1234"
	req.Header.Set("Remote-User", "alice@example.com")
	if _, err := FromRequest(req, cfg); err != ErrUntrustedSource {
		t.Fatalf("expected ErrUntrustedSource, got %v", err)
	}
}

func parseCIDR(value string) ([]*net.IPNet, error) {
	_, network, err := net.ParseCIDR(value)
	if err != nil {
		return nil, err
	}
	return []*net.IPNet{network}, nil
}
