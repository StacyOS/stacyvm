package worker

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/workerproto"
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

func TestRPCClientMTLSConformance(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey := writeTestCA(t, dir, "worker-ca")
	serverCert, serverKey := writeSignedTestCert(t, dir, "worker", caCert, caKey, testCertOptions{
		CommonName: "worker-a.internal",
		DNSNames:   []string{"worker-a.internal"},
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	})
	clientCert, clientKey := writeSignedTestCert(t, dir, "control-plane", caCert, caKey, testCertOptions{
		CommonName: "control-plane",
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	})

	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(context.Background(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn mock: %v", err)
	}

	serverTLS := TLSConfig{
		Enabled:        true,
		ServerCertFile: serverCert,
		ServerKeyFile:  serverKey,
		ClientCAFile:   caCert,
	}
	tlsConfig, err := serverTLS.ServerConfig()
	if err != nil {
		t.Fatalf("server tls config: %v", err)
	}
	server := httptest.NewUnstartedServer((&RPCServer{
		WorkerID: "worker-a",
		Token:    "worker-secret",
		Registry: registry,
	}).Handler())
	server.TLS = tlsConfig
	server.StartTLS()
	defer server.Close()

	client := RPCClient{
		BaseURL:  server.URL,
		WorkerID: "worker-a",
		Token:    "worker-secret",
		RPCTLS: TLSConfig{
			Enabled:        true,
			CAFile:         caCert,
			ClientCertFile: clientCert,
			ClientKeyFile:  clientKey,
			ServerName:     "worker-a.internal",
		},
	}
	result, err := client.Status(context.Background(), "req-mtls", workerproto.StatusParams{
		SandboxID: "sb-control-plane",
		RuntimeID: runtimeID,
		Provider:  "mock",
	})
	if err != nil {
		t.Fatalf("status over mtls: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || result.WorkerID != "worker-a" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

type testCertOptions struct {
	CommonName  string
	DNSNames    []string
	ExtKeyUsage []x509.ExtKeyUsage
}

func writeTestCA(t *testing.T, dir, name string) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	path := filepath.Join(dir, name+".crt")
	writePEMFile(t, path, "CERTIFICATE", der)
	return path, key
}

func writeSignedTestCert(t *testing.T, dir, name string, caCertPath string, caKey *rsa.PrivateKey, opts testCertOptions) (string, string) {
	t.Helper()
	caCert := readCert(t, caCertPath)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate %s key: %v", name, err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: opts.CommonName},
		DNSNames:     opts.DNSNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  opts.ExtKeyUsage,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create %s cert: %v", name, err)
	}
	certPath := filepath.Join(dir, name+".crt")
	keyPath := filepath.Join(dir, name+".key")
	writePEMFile(t, certPath, "CERTIFICATE", der)
	writePEMFile(t, keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
	return certPath, keyPath
}

func readCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("missing cert PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func writePEMFile(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()
	if err := pem.Encode(file, &pem.Block{Type: typ, Bytes: der}); err != nil {
		t.Fatalf("encode pem: %v", err)
	}
}
