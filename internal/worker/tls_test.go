package worker

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTLSConfigServerConfigRequiresCertPair(t *testing.T) {
	_, err := (TLSConfig{Enabled: true}).ServerConfig()
	if err == nil || !strings.Contains(err.Error(), "server cert and key") {
		t.Fatalf("expected server cert/key error, got %v", err)
	}
}

func TestTLSConfigHTTPClientRequiresClientCertPair(t *testing.T) {
	_, err := (TLSConfig{
		Enabled:        true,
		ClientCertFile: "/tmp/client.crt",
	}).HTTPClient(&http.Client{Timeout: 5 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "client cert and key") {
		t.Fatalf("expected client cert/key error, got %v", err)
	}
}

func TestTLSConfigHTTPClientPreservesDefaultTimeout(t *testing.T) {
	client, err := (TLSConfig{Enabled: true, InsecureSkipVerify: true}).HTTPClient(&http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("http client: %v", err)
	}
	if client.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", client.Timeout)
	}
	if client.Transport == nil {
		t.Fatal("transport is nil")
	}
}

func TestLoadCertPoolRejectsInvalidPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not pem"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := loadCertPool(path)
	if err == nil || !strings.Contains(err.Error(), "no PEM certificates") {
		t.Fatalf("expected PEM error, got %v", err)
	}
}
