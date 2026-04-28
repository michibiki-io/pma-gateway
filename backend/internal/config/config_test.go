package config

import (
	"strings"
	"testing"
)

func TestLoadDefaultsUseRootPublicBasePath(t *testing.T) {
	t.Setenv("PMA_GATEWAY_DEV_INSECURE_EPHEMERAL_KEY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Paths.PublicBasePath; got != "" {
		t.Fatalf("public base = %q", got)
	}
	if got := cfg.Paths.PublicEntryPath(); got != "/" {
		t.Fatalf("entry path = %q", got)
	}
	if got := cfg.Paths.PMABasePath(); got != "/_pma/" {
		t.Fatalf("pma path = %q", got)
	}
	if got := cfg.Paths.APIBasePath(); got != "/_api/v1" {
		t.Fatalf("api path = %q", got)
	}
	if got := cfg.Paths.SignonURL(); got != "/_signon.php" {
		t.Fatalf("signon path = %q", got)
	}
}

func TestPathsGenerateSubpathSafeDefaults(t *testing.T) {
	paths := Paths{
		PublicBasePath: "/dbadmin",
		PMAPath:        "/_pma",
		FrontendPath:   "/_gateway",
		APIPath:        "/_api",
		SignonPath:     "/_signon.php",
	}
	if got := paths.PublicEntryPath(); got != "/dbadmin/" {
		t.Fatalf("entry path = %q", got)
	}
	if got := paths.PMABasePath(); got != "/dbadmin/_pma/" {
		t.Fatalf("pma path = %q", got)
	}
	if got := paths.APIBasePath(); got != "/dbadmin/_api/v1" {
		t.Fatalf("api path = %q", got)
	}
	if got := paths.SignonURL(); got != "/dbadmin/_signon.php" {
		t.Fatalf("signon path = %q", got)
	}
}

func TestPathsGenerateRootBaseWithoutDoubleSlashes(t *testing.T) {
	paths := Paths{
		PublicBasePath: normalizePublicBase("/"),
		PMAPath:        normalizeSubpath("/_pma"),
		FrontendPath:   normalizeSubpath("/_gateway"),
		APIPath:        normalizeSubpath("/_api"),
		SignonPath:     normalizeSubpath("/_signon.php"),
	}
	if got := paths.PublicEntryPath(); got != "/" {
		t.Fatalf("entry path = %q", got)
	}
	if got := paths.PMABasePath(); got != "/_pma/" {
		t.Fatalf("pma path = %q", got)
	}
	if got := paths.APIBasePath(); got != "/_api/v1" {
		t.Fatalf("api path = %q", got)
	}
}

func TestParseBootstrapDocumentResolvesEnvAndSecretReferences(t *testing.T) {
	t.Setenv("BOOTSTRAP_DEV_DB_USER", "admin_user")
	t.Setenv("BOOTSTRAP_DEV_DB_PASSWORD", "fixture-bootstrap-password")

	doc, err := ParseBootstrapDocument(`{
		"credentials":[
			{
				"id":"dev-admin",
				"name":"Development Admin",
				"dbHost":"mariadb",
				"dbPort":3306,
				"dbUser":"env:BOOTSTRAP_DEV_DB_USER",
				"dbPassword":"secret:BOOTSTRAP_DEV_DB_PASSWORD",
				"enabled":true
			}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Credentials[0].DBUser; got != "admin_user" {
		t.Fatalf("dbUser = %q", got)
	}
	if got := doc.Credentials[0].DBPassword; got != "fixture-bootstrap-password" {
		t.Fatalf("dbPassword = %q", got)
	}
}

func TestParseBootstrapDocumentRejectsMissingSecretReference(t *testing.T) {
	_, err := ParseBootstrapDocument(`{
		"credentials":[
			{
				"id":"dev-admin",
				"name":"Development Admin",
				"dbHost":"mariadb",
				"dbPort":3306,
				"dbUser":"admin_user",
				"dbPassword":"secret:MISSING_BOOTSTRAP_PASSWORD",
				"enabled":true
			}
		]
	}`)
	if err == nil {
		t.Fatal("expected error for missing bootstrap secret reference")
	}
	if !strings.Contains(err.Error(), "MISSING_BOOTSTRAP_PASSWORD") {
		t.Fatalf("unexpected error: %v", err)
	}
}
