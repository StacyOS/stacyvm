package worker

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/workerproto"
)

type RPCServer struct {
	WorkerID     string
	Token        string
	Registry     *providers.Registry
	LeaseRenewer LeaseRenewer
	draining     atomic.Bool
}

type LeaseRenewer interface {
	RenewLease(ctx context.Context, resourceID, ttl string) (workerproto.LeaseToken, error)
}

func (s *RPCServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleRPC)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "worker_id": s.WorkerID})
	})
	return mux
}

func (s *RPCServer) Draining() bool {
	if s == nil {
		return false
	}
	return s.draining.Load()
}

func (s *RPCServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, httputil.CodeBadRequest, "method not allowed")
		return
	}
	if !s.authenticate(r) {
		httputil.WriteError(w, http.StatusUnauthorized, httputil.CodeUnauth, "invalid or missing worker RPC credentials")
		return
	}
	var req workerproto.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid worker RPC request")
		return
	}
	if req.WorkerID != s.WorkerID {
		httputil.WriteError(w, http.StatusForbidden, httputil.CodeUnauth, "worker RPC request targets a different worker")
		return
	}
	if err := workerproto.ValidateRequest(req); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, workerproto.Response{
			ID:       req.ID,
			WorkerID: s.WorkerID,
			Error:    err.Error(),
		})
		return
	}

	switch req.Method {
	case workerproto.MethodStatus:
		s.handleStatus(w, r.Context(), req)
	case workerproto.MethodRenewLease:
		s.handleRenewLease(w, r.Context(), req)
	case workerproto.MethodSpawn:
		if s.draining.Load() {
			httputil.WriteJSON(w, http.StatusServiceUnavailable, workerproto.Response{
				ID:       req.ID,
				WorkerID: s.WorkerID,
				Error:    "worker is draining",
			})
			return
		}
		s.handleSpawn(w, r.Context(), req)
	case workerproto.MethodDestroy:
		s.handleDestroy(w, r.Context(), req)
	case workerproto.MethodShutdown:
		s.draining.Store(true)
		httputil.WriteJSON(w, http.StatusOK, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID})
	default:
		httputil.WriteJSON(w, http.StatusNotImplemented, workerproto.Response{
			ID:       req.ID,
			WorkerID: s.WorkerID,
			Error:    "worker RPC method is not implemented by this worker runtime",
		})
	}
}

func (s *RPCServer) handleStatus(w http.ResponseWriter, ctx context.Context, req workerproto.Request) {
	var params workerproto.StatusParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	if s.Registry == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: "provider registry unavailable"})
		return
	}
	provider, err := s.Registry.Get(params.Provider)
	if err != nil {
		httputil.WriteJSON(w, http.StatusNotFound, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	runtimeID := strings.TrimSpace(params.RuntimeID)
	if runtimeID == "" {
		runtimeID = params.SandboxID
	}
	status, err := provider.Status(ctx, runtimeID)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, providers.ErrSandboxNotFound) {
			code = http.StatusNotFound
		}
		httputil.WriteJSON(w, code, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	result, _ := json.Marshal(workerproto.StatusResult{
		SandboxID: params.SandboxID,
		State:     status.State,
		Provider:  provider.Name(),
		WorkerID:  s.WorkerID,
	})
	httputil.WriteJSON(w, http.StatusOK, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Result: result})
}

func (s *RPCServer) handleSpawn(w http.ResponseWriter, ctx context.Context, req workerproto.Request) {
	if err := validateLeaseToken(req.Lease, s.WorkerID); err != nil {
		httputil.WriteJSON(w, http.StatusForbidden, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	var params workerproto.SpawnParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	if params.SandboxID != req.Lease.ResourceID {
		httputil.WriteJSON(w, http.StatusForbidden, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: "lease resource does not match spawn request"})
		return
	}
	if s.Registry == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: "provider registry unavailable"})
		return
	}
	provider, err := s.Registry.Get(params.Provider)
	if err != nil {
		httputil.WriteJSON(w, http.StatusNotFound, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	runtimeID, err := provider.Spawn(ctx, providers.SpawnOptions{
		Image:    params.Image,
		MemoryMB: params.MemoryMB,
		VCPUs:    params.VCPUs,
		Metadata: params.Metadata,
	})
	if err != nil {
		httputil.WriteJSON(w, http.StatusInternalServerError, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	result, _ := json.Marshal(workerproto.SpawnResult{
		SandboxID: params.SandboxID,
		RuntimeID: runtimeID,
		State:     "running",
		Provider:  provider.Name(),
		WorkerID:  s.WorkerID,
		Metadata:  params.Metadata,
	})
	httputil.WriteJSON(w, http.StatusOK, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Result: result})
}

func (s *RPCServer) handleDestroy(w http.ResponseWriter, ctx context.Context, req workerproto.Request) {
	if err := validateLeaseToken(req.Lease, s.WorkerID); err != nil {
		httputil.WriteJSON(w, http.StatusForbidden, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	var params workerproto.DestroyParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	if params.SandboxID != req.Lease.ResourceID {
		httputil.WriteJSON(w, http.StatusForbidden, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: "lease resource does not match destroy request"})
		return
	}
	if s.Registry == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: "provider registry unavailable"})
		return
	}
	provider, err := s.Registry.Get(params.Provider)
	if err != nil {
		httputil.WriteJSON(w, http.StatusNotFound, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	runtimeID := strings.TrimSpace(params.RuntimeID)
	if runtimeID == "" {
		runtimeID = params.SandboxID
	}
	if err := provider.Destroy(ctx, runtimeID); err != nil && !errors.Is(err, providers.ErrSandboxNotFound) {
		httputil.WriteJSON(w, http.StatusInternalServerError, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID})
}

func (s *RPCServer) handleRenewLease(w http.ResponseWriter, ctx context.Context, req workerproto.Request) {
	if err := validateLeaseToken(req.Lease, s.WorkerID); err != nil {
		httputil.WriteJSON(w, http.StatusForbidden, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	var params workerproto.RenewLeaseParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	if params.ResourceID != req.Lease.ResourceID {
		httputil.WriteJSON(w, http.StatusForbidden, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: "lease resource does not match renewal request"})
		return
	}
	if s.LeaseRenewer == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: "lease renewer unavailable"})
		return
	}
	lease, err := s.LeaseRenewer.RenewLease(ctx, params.ResourceID, params.TTL)
	if err != nil {
		httputil.WriteJSON(w, http.StatusConflict, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Error: err.Error()})
		return
	}
	result, _ := json.Marshal(workerproto.RenewLeaseResult{Lease: lease})
	httputil.WriteJSON(w, http.StatusOK, workerproto.Response{ID: req.ID, WorkerID: s.WorkerID, Result: result})
}

func validateLeaseToken(token *workerproto.LeaseToken, workerID string) error {
	if token == nil {
		return errors.New("lease token is required")
	}
	if token.HolderID != workerID {
		return errors.New("lease holder does not match worker")
	}
	if strings.TrimSpace(token.ResourceID) == "" {
		return errors.New("lease resource is required")
	}
	if token.ExpiresAt.Before(time.Now().UTC()) {
		return errors.New("lease token is expired")
	}
	return nil
}

func (s *RPCServer) authenticate(r *http.Request) bool {
	if strings.TrimSpace(s.Token) == "" || strings.TrimSpace(s.WorkerID) == "" {
		return false
	}
	if r.Header.Get("X-Worker-ID") != s.WorkerID {
		return false
	}
	token := r.Header.Get("X-Worker-Token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) == 1
}

func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
