package routes

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/store"
	gossh "golang.org/x/crypto/ssh"
)

type fakeCertSandboxStore struct {
	rec *store.SandboxRecord
	err error
}

func (f fakeCertSandboxStore) GetSandbox(context.Context, string) (*store.SandboxRecord, error) {
	return f.rec, f.err
}

func newTestCA(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	s, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	return s
}

func TestSSHCertRoutesIssuesCertForOwner(t *testing.T) {
	ca := newTestCA(t)
	store := fakeCertSandboxStore{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice"}}
	router := NewSSHCertRoutes(ca, store).Routes()

	body, _ := json.Marshal(map[string]any{"public_key": testSSHAuthorizedKey(t), "sandbox_id": "sb1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sshReqWithIdentity(http.MethodPost, "/", body, middleware.AuthIdentity{Subject: "alice"}))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Certificate string `json:"certificate"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(resp.Certificate))
	if err != nil {
		t.Fatalf("returned certificate not parseable: %v", err)
	}
	if _, ok := pub.(*gossh.Certificate); !ok {
		t.Fatalf("returned key is not a certificate: %T", pub)
	}
}

func TestSSHCertRoutesDeniesStranger(t *testing.T) {
	ca := newTestCA(t)
	st := fakeCertSandboxStore{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "bob", TenantID: "other"}}
	router := NewSSHCertRoutes(ca, st).Routes()

	body, _ := json.Marshal(map[string]any{"public_key": testSSHAuthorizedKey(t), "sandbox_id": "sb1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sshReqWithIdentity(http.MethodPost, "/", body, middleware.AuthIdentity{Subject: "alice", TenantID: "acme"}))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
