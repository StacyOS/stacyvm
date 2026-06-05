package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/httputil"
)

// WorkerTokenIssuerRoutes provides a centralized endpoint that lets the
// control plane mint short-lived signed worker tokens on behalf of workers.
// Workers call this endpoint with their bootstrap credentials to receive a
// short-lived signed token without needing direct access to the signing key.
type WorkerTokenIssuerRoutes struct {
	signingKey string
}

func NewWorkerTokenIssuerRoutes(signingKey string) *WorkerTokenIssuerRoutes {
	return &WorkerTokenIssuerRoutes{signingKey: signingKey}
}

type IssueWorkerTokenRequest struct {
	WorkerID string   `json:"worker_id"`
	TTL      string   `json:"ttl"`      // e.g. "5m", "15m"
	Scopes   []string `json:"scopes"`   // optional subset of worker scopes
	Audience string   `json:"audience"` // "worker:control-plane" or "worker:rpc"
}

type IssueWorkerTokenResponse struct {
	Token     string    `json:"token"`
	WorkerID  string    `json:"worker_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IssueToken mints a short-lived signed worker token.
// Requires admin auth (protected by the caller's middleware chain).
func (r *WorkerTokenIssuerRoutes) IssueToken(w http.ResponseWriter, req *http.Request) {
	if r.signingKey == "" {
		httputil.WriteError(w, http.StatusServiceUnavailable, httputil.CodeUnavailable,
			"worker signing key is not configured; set auth.worker_signing_key to enable centralized token issuance")
		return
	}

	var body IssueWorkerTokenRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	workerID := strings.TrimSpace(body.WorkerID)
	if workerID == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "worker_id is required")
		return
	}

	ttlStr := strings.TrimSpace(body.TTL)
	if ttlStr == "" {
		ttlStr = "5m"
	}
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil || ttl <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid ttl; use a Go duration like 5m or 15m")
		return
	}
	if ttl > middleware.MaxWorkerTokenTTL {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest,
			"ttl exceeds maximum allowed worker token lifetime of 15m")
		return
	}

	audience := strings.TrimSpace(body.Audience)
	if audience == "" {
		audience = middleware.WorkerTokenAudienceControlPlane
	}
	if audience != middleware.WorkerTokenAudienceControlPlane && audience != middleware.WorkerTokenAudienceRPC {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest,
			"audience must be worker:control-plane or worker:rpc")
		return
	}

	// Validate requested scopes up-front: only worker:* scopes may be issued.
	// This prevents escalation through the issuer even if the caller holds admin credentials.
	if len(body.Scopes) > 0 {
		for _, s := range body.Scopes {
			if !strings.HasPrefix(strings.TrimSpace(s), "worker:") {
				httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest,
					"only worker:* scopes may be issued; scope "+strconv.Quote(s)+" is not permitted")
				return
			}
		}
	}

	tokenID, err := middleware.NewWorkerTokenID()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, "failed to generate token ID")
		return
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	claims := middleware.WorkerTokenClaims{
		WorkerID:  workerID,
		TokenID:   tokenID,
		Audience:  audience,
		Scopes:    body.Scopes,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}

	token, err := middleware.SignWorkerToken(r.signingKey, claims)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, "failed to sign worker token")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, IssueWorkerTokenResponse{
		Token:     token,
		WorkerID:  workerID,
		ExpiresAt: expiresAt,
	})
}
