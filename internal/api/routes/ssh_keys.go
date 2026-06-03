package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"
)

type sshKeyStore interface {
	CreateSSHKey(ctx context.Context, key *store.SSHKeyRecord) error
	ListSSHKeysByOwner(ctx context.Context, ownerID string) ([]*store.SSHKeyRecord, error)
	DeleteSSHKey(ctx context.Context, id string) error
}

// SSHKeyRoutes manages a caller's registered SSH public keys, used by the SSH
// gateway for publickey authentication.
type SSHKeyRoutes struct {
	store sshKeyStore
}

func NewSSHKeyRoutes(st sshKeyStore) *SSHKeyRoutes {
	return &SSHKeyRoutes{store: st}
}

func (sr *SSHKeyRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", sr.List)
	r.Post("/", sr.Create)
	r.Delete("/{keyID}", sr.Delete)
	return r
}

type registerSSHKeyRequest struct {
	PublicKey string `json:"public_key"`
	Label     string `json:"label"`
}

// sshKeyOwner mirrors sandbox ownership resolution (X-User-ID header, falling
// back to the OIDC subject) so a key authorizes the same sandboxes its owner
// can create.
func sshKeyOwner(r *http.Request) (ownerID, tenantID string) {
	identity := middleware.AuthIdentityFromContext(r.Context())
	ownerID = strings.TrimSpace(r.Header.Get("X-User-ID"))
	if ownerID == "" {
		ownerID = identity.Subject
	}
	return ownerID, identity.TenantID
}

func (sr *SSHKeyRoutes) Create(w http.ResponseWriter, r *http.Request) {
	var req registerSSHKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid public key")
		return
	}

	ownerID, tenantID := sshKeyOwner(r)
	rec := &store.SSHKeyRecord{
		ID:          uuid.NewString(),
		OwnerID:     ownerID,
		TenantID:    tenantID,
		Fingerprint: gossh.FingerprintSHA256(pub),
		PublicKey:   strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pub))),
		Label:       strings.TrimSpace(req.Label),
	}
	if err := sr.store.CreateSSHKey(r.Context(), rec); err != nil {
		if errors.Is(err, store.ErrConflict) {
			httputil.WriteError(w, http.StatusConflict, httputil.CodeConflict, "ssh key already registered")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, rec)
}

func (sr *SSHKeyRoutes) List(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := sshKeyOwner(r)
	keys, err := sr.store.ListSSHKeysByOwner(r.Context(), ownerID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if keys == nil {
		keys = []*store.SSHKeyRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ssh_keys": keys})
}

func (sr *SSHKeyRoutes) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "keyID")
	ownerID, _ := sshKeyOwner(r)

	// Enforce ownership: callers may only delete their own keys.
	keys, err := sr.store.ListSSHKeysByOwner(r.Context(), ownerID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	owned := false
	for _, k := range keys {
		if k.ID == id {
			owned = true
			break
		}
	}
	if !owned {
		httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "ssh key not found")
		return
	}
	if err := sr.store.DeleteSSHKey(r.Context(), id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
