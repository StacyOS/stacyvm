package routes

import (
	"bytes"
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

type fakeSSHKeyStore struct {
	keys map[string]*store.SSHKeyRecord
}

func newFakeSSHKeyStore() *fakeSSHKeyStore {
	return &fakeSSHKeyStore{keys: map[string]*store.SSHKeyRecord{}}
}

func (f *fakeSSHKeyStore) CreateSSHKey(_ context.Context, k *store.SSHKeyRecord) error {
	for _, e := range f.keys {
		if e.Fingerprint == k.Fingerprint {
			return store.ConflictError("ssh key already exists")
		}
	}
	f.keys[k.ID] = k
	return nil
}

func (f *fakeSSHKeyStore) ListSSHKeysByOwner(_ context.Context, owner string) ([]*store.SSHKeyRecord, error) {
	var out []*store.SSHKeyRecord
	for _, e := range f.keys {
		if e.OwnerID == owner {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeSSHKeyStore) GetSSHKeyByFingerprint(_ context.Context, fp string) (*store.SSHKeyRecord, error) {
	for _, e := range f.keys {
		if e.Fingerprint == fp {
			return e, nil
		}
	}
	return nil, store.NotFoundError("ssh key", fp)
}

func (f *fakeSSHKeyStore) DeleteSSHKey(_ context.Context, id string) error {
	if _, ok := f.keys[id]; !ok {
		return store.NotFoundError("ssh key", id)
	}
	delete(f.keys, id)
	return nil
}

func testSSHAuthorizedKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("new public key: %v", err)
	}
	return string(gossh.MarshalAuthorizedKey(sshPub))
}

func sshReqWithIdentity(method, target string, body []byte, id middleware.AuthIdentity) *http.Request {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, bytes.NewReader(body))
	}
	return r.WithContext(middleware.WithAuthIdentity(r.Context(), id))
}

func TestSSHKeyRoutesCreateStoresOwnerAndFingerprint(t *testing.T) {
	st := newFakeSSHKeyStore()
	router := NewSSHKeyRoutes(st).Routes()

	body, _ := json.Marshal(map[string]string{"public_key": testSSHAuthorizedKey(t), "label": "laptop"})
	id := middleware.AuthIdentity{Subject: "alice", TenantID: "acme"}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sshReqWithIdentity(http.MethodPost, "/", body, id))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	stored, _ := st.ListSSHKeysByOwner(context.Background(), "alice")
	if len(stored) != 1 {
		t.Fatalf("stored keys = %d, want 1", len(stored))
	}
	if stored[0].TenantID != "acme" || stored[0].Fingerprint == "" {
		t.Fatalf("stored key = %+v, want tenant=acme and a fingerprint", stored[0])
	}

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, sshReqWithIdentity(http.MethodGet, "/", nil, id))
	if rec2.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec2.Code)
	}
}

func TestSSHKeyRoutesRejectsInvalidKey(t *testing.T) {
	router := NewSSHKeyRoutes(newFakeSSHKeyStore()).Routes()
	body, _ := json.Marshal(map[string]string{"public_key": "not-a-key"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sshReqWithIdentity(http.MethodPost, "/", body, middleware.AuthIdentity{Subject: "alice"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSSHKeyRoutesDeleteUnownedReturns404(t *testing.T) {
	st := newFakeSSHKeyStore()
	st.keys["k1"] = &store.SSHKeyRecord{ID: "k1", OwnerID: "bob", Fingerprint: "fp"}
	router := NewSSHKeyRoutes(st).Routes()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sshReqWithIdentity(http.MethodDelete, "/k1", nil, middleware.AuthIdentity{Subject: "alice"}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if _, ok := st.keys["k1"]; !ok {
		t.Fatal("bob's key was deleted by alice")
	}
}
