package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSupportBundleRedactsSecrets(t *testing.T) {
	input := map[string]interface{}{
		"api_key": "sk-super-secret-value",
		"nested": map[string]interface{}{
			"token": "Bearer abcdefghijklmnop",
			"url":   "https://user:password@example.com/path",
		},
		"message": "X-Admin-API-Key: admin-secret-value",
	}

	redacted := redactMap(input)
	data, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, secret := range []string{"sk-super-secret-value", "abcdefghijklmnop", "password", "admin-secret-value"} {
		if strings.Contains(body, secret) {
			t.Fatalf("support redaction leaked %q in %s", secret, body)
		}
	}
	if !strings.Contains(body, "[REDACTED]") {
		t.Fatalf("expected redaction marker in %s", body)
	}
}

func TestWriteSupportBundleRedactsFinalJSON(t *testing.T) {
	path := t.TempDir() + "/support.json"
	bundle := supportBundle{
		Version: "test",
		CollectionErrors: []string{
			"server returned bearer abcdefghijklmnop",
			"database url postgres://user:pass@example.com/db",
		},
	}
	if err := writeSupportBundle(path, bundle); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, secret := range []string{"abcdefghijklmnop", "user:pass"} {
		if strings.Contains(body, secret) {
			t.Fatalf("final support bundle leaked %q in %s", secret, body)
		}
	}
}
