package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michibiki-io/pma-gateway/backend/internal/auditmeta"
	"github.com/michibiki-io/pma-gateway/backend/internal/config"
	pmacrypto "github.com/michibiki-io/pma-gateway/backend/internal/crypto"
	"github.com/michibiki-io/pma-gateway/backend/internal/storage"
	"go.uber.org/zap"
)

func testRouter(t *testing.T) (http.Handler, *storage.Store) {
	t.Helper()
	_, network, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Paths: config.Paths{
			PublicBasePath: "/dbadmin",
			PMAPath:        "/_pma",
			FrontendPath:   "/_gateway",
			APIPath:        "/_api",
			SignonPath:     "/_signon.php",
		},
		MasterKey:              []byte("0123456789abcdef0123456789abcdef"),
		InternalSharedSecret:   "test-secret",
		TicketTTL:              time.Minute,
		CredentialTestTimeout:  100 * time.Millisecond,
		UserHeader:             "Remote-User",
		GroupsHeader:           "Remote-Groups",
		GroupsSeparator:        ",",
		AdminUsers:             map[string]struct{}{"admin@example.com": {}},
		AdminGroups:            map[string]struct{}{"db-admins": {}},
		AppCheckMode:           "disabled",
		AppCheckVerifiedHeader: "X-AppCheck-Verified",
		TrustedProxyCIDRs:      []*net.IPNet{network},
		TrustProxyHeaders:      true,
	}
	cipher, err := pmacrypto.NewCipher(cfg.MasterKey)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(context.Background(), storage.Options{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "test.db")}, cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewRouter(cfg, store, zap.NewNop()), store
}

func TestAdminAuthorizationRejectsNonAdmin(t *testing.T) {
	router, _ := testRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/dbadmin/_api/v1/admin/credentials", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	req.Header.Set("Remote-User", "bob@example.com")
	req.Header.Set("Remote-Groups", "db-users")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePMASessionReturnsRedirectOnly(t *testing.T) {
	router, store := testRouter(t)
	ctx := context.Background()
	_, err := store.CreateCredential(ctx, storage.CredentialInput{
		ID: "dev", Name: "Development", DBHost: "mariadb", DBPort: 3306,
		DBUser: "root", DBPassword: "fixture-db-password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateMapping(ctx, storage.MappingInput{SubjectType: "user", Subject: "alice@example.com", CredentialID: "dev"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/dbadmin/_api/v1/pma/sessions", bytes.NewBufferString(`{"credentialId":"dev"}`))
	req.RemoteAddr = "10.0.0.5:12345"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Remote-User", "alice@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body["redirectUrl"], "/dbadmin/_signon.php?ticket=") {
		t.Fatalf("unexpected redirect URL: %q", body["redirectUrl"])
	}
	if strings.Contains(rec.Body.String(), "fixture-db-password") {
		t.Fatal("response exposed the DB password")
	}
}

func TestUnifiedEntryRedirectsToPhpMyAdminBase(t *testing.T) {
	router, _ := testRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/dbadmin/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/dbadmin/_pma/" {
		t.Fatalf("Location = %q", location)
	}
}

func TestCredentialConnectionTestReturnsFailureWithoutServerError(t *testing.T) {
	router, _ := testRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/dbadmin/_api/v1/admin/credentials/test", bytes.NewBufferString(`{"dbHost":"127.0.0.1","dbPort":1,"dbUser":"root","dbPassword":"secret"}`))
	req.RemoteAddr = "10.0.0.5:12345"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Remote-User", "admin@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Success {
		t.Fatal("expected connection test failure for an unavailable target")
	}
}

func TestFrontendConfigIncludesVersionMetadata(t *testing.T) {
	router, _ := testRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/dbadmin/_gateway/config.js", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/javascript") {
		t.Fatalf("content-type = %q", contentType)
	}
	body := rec.Body.String()
	for _, fragment := range []string{
		`"version"`,
		`"appVersion"`,
		`"appDisplayVersion"`,
		`"phpMyAdminVersion"`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("config.js missing fragment %q in body %s", fragment, body)
		}
	}
}

func TestAuditFilterOptionsExposeMetadataAndRecentActors(t *testing.T) {
	router, store := testRouter(t)
	for _, event := range []storage.AuditEvent{
		{ID: "audit_1", Timestamp: "2001-01-01T00:00:01Z", Actor: "alice@example.com", Action: auditmeta.ActionCredentialCreate, TargetType: auditmeta.TargetTypeCredential, Result: "success"},
		{ID: "audit_2", Timestamp: "2001-01-01T00:00:02Z", Actor: "bob@example.com", Action: auditmeta.ActionCredentialUpdate, TargetType: auditmeta.TargetTypeCredential, Result: "success"},
		{ID: "audit_3", Timestamp: "2001-01-01T00:00:03Z", Actor: "alice@example.com", Action: auditmeta.ActionAuditView, TargetType: auditmeta.TargetTypeAudit, Result: "success"},
	} {
		if err := store.InsertAuditEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/dbadmin/_api/v1/admin/audit-events/filter-options", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	req.Header.Set("Remote-User", "admin@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Actions          []string `json:"actions"`
		TargetTypes      []string `json:"targetTypes"`
		ActorSuggestions []string `json:"actorSuggestions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Actions) == 0 || body.Actions[0] != auditmeta.ActionBootstrapApply {
		t.Fatalf("unexpected actions: %+v", body.Actions)
	}
	if len(body.TargetTypes) == 0 || body.TargetTypes[0] != auditmeta.TargetTypeSystem {
		t.Fatalf("unexpected target types: %+v", body.TargetTypes)
	}
	if got, want := body.ActorSuggestions, []string{"alice@example.com", "bob@example.com"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("actor suggestions = %+v want %+v", got, want)
	}
}

func TestListAuditEventsFormatsTimestampsForDisplay(t *testing.T) {
	router, store := testRouter(t)
	if err := store.InsertAuditEvent(context.Background(), storage.AuditEvent{
		ID:         "audit_test",
		Timestamp:  "2001-01-01T01:00:00Z",
		Actor:      "admin@example.com",
		Action:     "audit.test",
		TargetType: auditmeta.TargetTypeAudit,
		Result:     "success",
		Message:    "test event",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dbadmin/_api/v1/admin/audit-events?page=1&pageSize=10", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	req.Header.Set("Remote-User", "admin@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Items []storage.AuditEvent `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("unexpected item count: %d", len(body.Items))
	}
	if body.Items[0].Timestamp != "2001-01-01 10:00:00 JST" {
		t.Fatalf("timestamp = %q", body.Items[0].Timestamp)
	}
}
