package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/httputil"
	stacyssh "github.com/StacyOs/stacyvm/internal/ssh"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
	gossh "golang.org/x/crypto/ssh"
)

type sshCertSandboxStore interface {
	GetSandbox(ctx context.Context, id string) (*store.SandboxRecord, error)
}

// SSHCertRoutes issues short-lived SSH user certificates bound to a sandbox,
// signed by the deployment's user CA. This powers the `stacy ssh` ephemeral
// flow: authenticate with the normal API, receive a 10-minute cert.
type SSHCertRoutes struct {
	ca        gossh.Signer
	sandboxes sshCertSandboxStore
}

func NewSSHCertRoutes(ca gossh.Signer, sandboxes sshCertSandboxStore) *SSHCertRoutes {
	return &SSHCertRoutes{ca: ca, sandboxes: sandboxes}
}

func (cr *SSHCertRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", cr.Issue)
	return r
}

type issueSSHCertRequest struct {
	PublicKey  string `json:"public_key"`
	SandboxID  string `json:"sandbox_id"`
	TTLSeconds int    `json:"ttl_seconds"`
}

func (cr *SSHCertRoutes) Issue(w http.ResponseWriter, r *http.Request) {
	var req issueSSHCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	sandboxID := strings.TrimSpace(req.SandboxID)
	if sandboxID == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "sandbox_id is required")
		return
	}
	userKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid public key")
		return
	}

	sb, err := cr.sandboxes.GetSandbox(r.Context(), sandboxID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "sandbox not found")
		return
	}

	ownerID, tenantID := sshKeyOwner(r)
	if !sshCertAuthorized(ownerID, tenantID, sb) {
		httputil.WriteError(w, http.StatusForbidden, httputil.CodeUnauth, "not authorized for sandbox")
		return
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	cert, err := stacyssh.SignUserCertificate(cr.ca, userKey, stacyssh.Identity{
		Subject:  ownerID,
		OwnerID:  ownerID,
		TenantID: tenantID,
	}, sandboxID, ttl)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"certificate": strings.TrimSpace(string(gossh.MarshalAuthorizedKey(cert))),
		"sandbox_id":  sandboxID,
	})
}

// sshCertAuthorized mirrors the gateway's owner/tenant authorization so cert
// issuance fails fast for sandboxes the caller cannot access.
func sshCertAuthorized(ownerID, tenantID string, sb *store.SandboxRecord) bool {
	switch {
	case sb.OwnerID != "" && sb.OwnerID == ownerID:
		return true
	case sb.TenantID != "" && tenantID != "" && sb.TenantID == tenantID:
		return true
	case sb.OwnerID == "" && sb.TenantID == "":
		return true
	default:
		return false
	}
}
