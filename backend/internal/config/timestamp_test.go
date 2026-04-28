package config

import (
	"encoding/base64"
	"testing"
)

func TestLoadTimestampDisplayConfigDefaults(t *testing.T) {
	t.Setenv("PMA_GATEWAY_MASTER_KEY_BASE64", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("PMA_GATEWAY_INTERNAL_SHARED_SECRET", "internal")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.FormatTimestamp("2001-01-01T01:00:00Z"); got != "2001-01-01 10:00:00 JST" {
		t.Fatalf("default formatted timestamp = %q", got)
	}
}

func TestLoadTimestampDisplayConfigOverrides(t *testing.T) {
	t.Setenv("PMA_GATEWAY_MASTER_KEY_BASE64", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("PMA_GATEWAY_INTERNAL_SHARED_SECRET", "internal")
	t.Setenv("PMA_GATEWAY_TIMESTAMP_FORMAT", "2006/01/02 15:04:05 MST")
	t.Setenv("PMA_GATEWAY_TIMESTAMP_TIMEZONE", "UTC")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.FormatTimestamp("2001-01-01T01:00:00Z"); got != "2001/01/01 01:00:00 UTC" {
		t.Fatalf("custom formatted timestamp = %q", got)
	}
}
