package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadMySQLDatabaseConfigFromComponents(t *testing.T) {
	t.Setenv("PMA_GATEWAY_MASTER_KEY_BASE64", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("PMA_GATEWAY_INTERNAL_SHARED_SECRET", "internal")
	t.Setenv("PMA_GATEWAY_DATABASE_DRIVER", "mysql")
	t.Setenv("PMA_GATEWAY_MYSQL_HOST", "gateway-mysql")
	t.Setenv("PMA_GATEWAY_MYSQL_PORT", "3306")
	t.Setenv("PMA_GATEWAY_MYSQL_DATABASE", "pma_gateway")
	t.Setenv("PMA_GATEWAY_MYSQL_USER", "pma_gateway")
	t.Setenv("PMA_GATEWAY_MYSQL_PASSWORD", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseDriver != "mysql" {
		t.Fatalf("driver = %q", cfg.DatabaseDriver)
	}
	if !strings.Contains(cfg.DatabaseDSN, "pma_gateway:secret@tcp(gateway-mysql:3306)/pma_gateway") {
		t.Fatalf("unexpected DSN: %q", cfg.DatabaseDSN)
	}
}
