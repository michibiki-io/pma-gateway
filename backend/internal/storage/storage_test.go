package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/michibiki-io/pma-gateway/backend/internal/auditmeta"
	"github.com/michibiki-io/pma-gateway/backend/internal/config"
	pmacrypto "github.com/michibiki-io/pma-gateway/backend/internal/crypto"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	cipher, err := pmacrypto.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), Options{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "test.db")}, cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func seedCredentialAndMapping(t *testing.T, store *Store) {
	t.Helper()
	_, err := store.CreateCredential(context.Background(), CredentialInput{
		ID: "dev-readonly", Name: "Development Readonly", DBHost: "mariadb", DBPort: 3306,
		DBUser: "readonly", DBPassword: "fixture-readonly-password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateMapping(context.Background(), MappingInput{
		SubjectType: "group", Subject: "db-users", CredentialID: "dev-readonly",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMappingAuthorizationListsAllowedCredentials(t *testing.T) {
	store := newTestStore(t)
	seedCredentialAndMapping(t, store)
	items, err := store.AvailableCredentials(context.Background(), "alice@example.com", []string{"db-users"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "dev-readonly" {
		t.Fatalf("unexpected credentials: %+v", items)
	}
	ok, err := store.UserCanUseCredential(context.Background(), "bob@example.com", []string{"other"}, "dev-readonly")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("unmapped user was authorized")
	}
}

func TestSignonTicketRedeemAndReplayRejection(t *testing.T) {
	store := newTestStore(t)
	seedCredentialAndMapping(t, store)
	ticket, _, err := store.CreateSignonTicket(context.Background(), "alice@example.com", "dev-readonly", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	redeemed, err := store.RedeemSignonTicket(context.Background(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if redeemed.DBPassword != "fixture-readonly-password" || redeemed.Actor != "alice@example.com" {
		t.Fatalf("unexpected redeemed credential: %+v", redeemed)
	}
	if _, err := store.RedeemSignonTicket(context.Background(), ticket); !errors.Is(err, ErrTicketUsed) {
		t.Fatalf("expected replay rejection, got %v", err)
	}
}

func TestAuditPaginationAndReset(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, actor := range []string{"alice@example.com", "bob@example.com", "alice@example.com"} {
		if err := store.InsertAuditEvent(ctx, AuditEvent{Actor: actor, Action: auditmeta.ActionCredentialAvailableList, TargetType: auditmeta.TargetTypeCredential, Result: "success"}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.ListAuditEvents(ctx, AuditFilter{Actor: "alice@example.com", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalItems != 2 || len(page.Items) != 2 {
		t.Fatalf("unexpected audit page: %+v", page)
	}
	reset, err := store.ResetAuditEvents(ctx, "admin@example.com", []string{"db-admins"}, "127.0.0.1", "test", "unit test")
	if err != nil {
		t.Fatal(err)
	}
	if reset.DeletedEvents != 3 {
		t.Fatalf("deleted events = %d", reset.DeletedEvents)
	}
	page, err = store.ListAuditEvents(ctx, AuditFilter{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalItems != 1 || page.Items[0].Action != auditmeta.ActionAuditReset {
		t.Fatalf("reset marker not visible: %+v", page)
	}
}

func TestListRecentAuditActorsReturnsDistinctMostRecentActors(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, event := range []AuditEvent{
		{ID: "audit_1", Timestamp: "2001-01-01T00:00:01Z", Actor: "alice@example.com", Action: auditmeta.ActionCredentialCreate, TargetType: auditmeta.TargetTypeCredential, Result: "success"},
		{ID: "audit_2", Timestamp: "2001-01-01T00:00:02Z", Actor: "bob@example.com", Action: auditmeta.ActionCredentialUpdate, TargetType: auditmeta.TargetTypeCredential, Result: "success"},
		{ID: "audit_3", Timestamp: "2001-01-01T00:00:03Z", Actor: "alice@example.com", Action: auditmeta.ActionAuditView, TargetType: auditmeta.TargetTypeAudit, Result: "success"},
	} {
		if err := store.InsertAuditEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	actors, err := store.ListRecentAuditActors(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := actors, []string{"alice@example.com", "bob@example.com"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("actors = %+v want %+v", got, want)
	}
}

func TestBootstrapImport(t *testing.T) {
	store := newTestStore(t)
	boot := config.Bootstrap{
		Enabled: true,
		Mode:    "first-run",
		RawJSON: `{"credentials":[{"id":"dev-root","name":"Development Root","dbHost":"mariadb","dbPort":3306,"dbUser":"root","dbPassword":"fixture-bootstrap-password","enabled":true}],"mappings":[{"subjectType":"user","subject":"alice@example.com","credentialId":"dev-root"}]}`,
	}
	result, err := store.ApplyBootstrap(context.Background(), boot)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.Credentials != 1 || result.Mappings != 1 {
		t.Fatalf("unexpected bootstrap result: %+v", result)
	}
	items, err := store.AvailableCredentials(context.Background(), "alice@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "dev-root" {
		t.Fatalf("bootstrap mapping not available: %+v", items)
	}
}

func TestCredentialConnectionTestCanReuseStoredPassword(t *testing.T) {
	store := newTestStore(t)
	seedCredentialAndMapping(t, store)

	err := store.TestCredentialConnection(context.Background(), CredentialTestInput{
		ExistingCredentialID: "dev-readonly",
		DBHost:               "127.0.0.1",
		DBPort:               1,
		DBUser:               "readonly",
		Timeout:              100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected connection failure for an unavailable target")
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("stored password was not reused: %v", err)
	}
}
