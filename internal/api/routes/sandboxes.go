package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type sandboxPolicyStore interface {
	ListPolicies(ctx context.Context, query store.PolicyQuery) ([]*store.PolicyRecord, error)
}

type SandboxRoutes struct {
	manager     *orchestrator.Manager
	policyStore sandboxPolicyStore
}

func NewSandboxRoutes(manager *orchestrator.Manager) *SandboxRoutes {
	return &SandboxRoutes{manager: manager}
}

func NewSandboxRoutesWithPolicy(manager *orchestrator.Manager, ps sandboxPolicyStore) *SandboxRoutes {
	return &SandboxRoutes{manager: manager, policyStore: ps}
}

func (s *SandboxRoutes) Routes() chi.Router {
	return s.RoutesWithScopeEnforcement(false)
}

// RoutesWithScopeEnforcement returns the router with scope checks enabled when
// enforce is true (used by the server when auth is configured).
func (s *SandboxRoutes) RoutesWithScopeEnforcement(enforce bool) chi.Router {
	r := chi.NewRouter()

	readScope := optionalScope(enforce, middleware.ScopeRead)
	apiScope := optionalScope(enforce, middleware.ScopeAPI)

	// Read-only operations: viewer, api, operator, admin.
	r.With(readScope).Get("/", s.List)
	r.With(readScope).Post("/admission", s.Admission)

	// Mutating operations: api, operator, admin (not viewer).
	// PolicyEnforcer runs before Create to enforce tenant image/provider/network rules.
	spawnChain := []func(http.Handler) http.Handler{apiScope}
	if s.policyStore != nil {
		spawnChain = append(spawnChain, middleware.PolicyEnforcer(s.policyStore))
	}
	r.With(spawnChain...).Post("/", s.Create)
	r.With(apiScope).Delete("/", s.Prune)

	r.Route("/{sandboxID}", func(r chi.Router) {
		// Read-only.
		r.With(readScope).Get("/", s.Get)
		r.With(readScope).Get("/files", s.ReadFile)
		r.With(readScope).Get("/files/list", s.ListFiles)
		r.With(readScope).Get("/files/stat", s.StatFile)
		r.With(readScope).Get("/files/glob", s.GlobFiles)
		r.With(readScope).Get("/logs", s.ConsoleLog)
		// Mutating.
		r.With(apiScope).Delete("/", s.Destroy)
		r.With(apiScope).Post("/extend", s.Extend)
		r.With(apiScope).Post("/exec", s.Exec)
		r.With(apiScope).Get("/exec/ws", s.ExecWebSocket)
		r.With(apiScope).Post("/files", s.WriteFile)
		r.With(apiScope).Delete("/files", s.DeleteFile)
		r.With(apiScope).Post("/files/move", s.MoveFile)
		r.With(apiScope).Post("/files/chmod", s.ChmodFile)
	})
	return r
}

// optionalScope returns RequireScope when enforce is true, otherwise a no-op.
func optionalScope(enforce bool, scope string) func(http.Handler) http.Handler {
	if enforce {
		return middleware.RequireScope(scope)
	}
	return func(next http.Handler) http.Handler { return next }
}

// Create creates a new sandbox.
//
//	@Summary		Create a sandbox
//	@Description	Spawn a new sandbox with the given configuration
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			request	body		orchestrator.SpawnRequest	true	"Spawn request"
//	@Success		201		{object}	orchestrator.Sandbox
//	@Failure		400		{object}	httputil.APIError
//	@Failure		429		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes [post]
func (s *SandboxRoutes) Create(w http.ResponseWriter, r *http.Request) {
	var req orchestrator.SpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	identity := middleware.AuthIdentityFromContext(r.Context())
	// Extract owner from X-User-ID header, falling back to OIDC subject.
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		req.OwnerID = userID
	} else if identity.Subject != "" && req.OwnerID == "" {
		req.OwnerID = identity.Subject
	}
	// Stamp the tenant from the caller's identity so sandboxes are scoped.
	req.TenantID = identity.TenantID

	sb, err := s.manager.Spawn(r.Context(), req)
	if err != nil {
		writeRouteError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, sb)
}

// Admission evaluates whether a spawn request would be admitted.
//
//	@Summary		Evaluate spawn admission
//	@Description	Return whether a spawn request would be allowed, queued, or denied by quota and scheduler limits
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			request	body		orchestrator.SpawnRequest	true	"Spawn request"
//	@Success		200		{object}	orchestrator.SpawnAdmissionDecision
//	@Failure		400		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/admission [post]
func (s *SandboxRoutes) Admission(w http.ResponseWriter, r *http.Request) {
	var req orchestrator.SpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		req.OwnerID = userID
	}

	decision, err := s.manager.EvaluateSpawnRequestAdmission(r.Context(), req)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, decision)
}

// List lists all active sandboxes.
//
//	@Summary		List sandboxes
//	@Description	Return all active sandboxes
//	@Tags			sandboxes
//	@Produce		json
//	@Success		200	{array}		orchestrator.Sandbox
//	@Failure		500	{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes [get]
func (s *SandboxRoutes) List(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := s.manager.List(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if sandboxes == nil {
		sandboxes = []*orchestrator.Sandbox{}
	}
	// Enforce tenant scoping: callers with a tenant identity only see their tenant's sandboxes.
	identity := middleware.AuthIdentityFromContext(r.Context())
	if identity.TenantID != "" {
		filtered := sandboxes[:0]
		for _, sb := range sandboxes {
			if sb.TenantID == identity.TenantID {
				filtered = append(filtered, sb)
			}
		}
		sandboxes = filtered
	}
	httputil.WriteJSON(w, http.StatusOK, sandboxes)
}

// Get returns a single sandbox by ID.
//
//	@Summary		Get a sandbox
//	@Description	Return a sandbox by its ID
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Success		200			{object}	orchestrator.Sandbox
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID} [get]
func (s *SandboxRoutes) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	sb, err := s.manager.Get(r.Context(), id)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	// Enforce tenant scoping.
	if tenantID := middleware.AuthIdentityFromContext(r.Context()).TenantID; tenantID != "" && sb.TenantID != tenantID {
		httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "sandbox not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sb)
}

// Destroy destroys a sandbox by ID.
//
//	@Summary		Destroy a sandbox
//	@Description	Destroy a sandbox and release its resources
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Success		200			{object}	StatusResponse
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID} [delete]
// checkTenantAccess fetches the sandbox and returns false (writing 404) if the
// caller's tenant does not match the sandbox's tenant. Callers with no tenant
// (API key without X-Tenant-ID) bypass the check.
func (s *SandboxRoutes) checkTenantAccess(w http.ResponseWriter, r *http.Request, sandboxID string) bool {
	tenantID := middleware.AuthIdentityFromContext(r.Context()).TenantID
	if tenantID == "" {
		return true
	}
	sb, err := s.manager.Get(r.Context(), sandboxID)
	if err != nil {
		writeRouteError(w, err)
		return false
	}
	if sb.TenantID != tenantID {
		httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "sandbox not found")
		return false
	}
	return true
}

func (s *SandboxRoutes) Destroy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	if err := s.manager.Destroy(r.Context(), id); err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
}

// Extend extends a sandbox's TTL.
//
//	@Summary		Extend sandbox TTL
//	@Description	Add additional time to a sandbox's expiration
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			request		body		object{ttl=string}	true	"TTL extension"
//	@Success		200			{object}	orchestrator.Sandbox
//	@Failure		400			{object}	httputil.APIError
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/extend [post]
func (s *SandboxRoutes) Extend(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	var req struct {
		TTL string `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if req.TTL == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "ttl is required")
		return
	}

	duration, err := time.ParseDuration(req.TTL)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid ttl format: "+err.Error())
		return
	}

	if duration <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "ttl must be positive")
		return
	}

	sb, err := s.manager.ExtendTTL(r.Context(), id, duration)
	if err != nil {
		writeRouteError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, sb)
}

// Exec executes a command in a sandbox.
//
//	@Summary		Execute a command
//	@Description	Run a command inside a sandbox. Set stream=true for streaming output.
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			sandboxID	path		string					true	"Sandbox ID"
//	@Param			request		body		orchestrator.ExecRequest	true	"Exec request"
//	@Success		200			{object}	orchestrator.ExecResult
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/exec [post]
func (s *SandboxRoutes) Exec(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	var req orchestrator.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if req.Stream {
		s.execStream(w, r, id, req)
		return
	}

	result, err := s.manager.Exec(r.Context(), id, req)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

func (s *SandboxRoutes) execStream(w http.ResponseWriter, r *http.Request, id string, req orchestrator.ExecRequest) {
	ch, err := s.manager.ExecStream(r.Context(), id, req)
	if err != nil {
		writeRouteError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)

	for chunk := range ch {
		enc.Encode(chunk)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// WriteFile writes a file into a sandbox.
//
//	@Summary		Write a file
//	@Description	Write content to a file inside a sandbox
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			sandboxID	path		string						true	"Sandbox ID"
//	@Param			request		body		orchestrator.FileWriteRequest	true	"File write request"
//	@Success		200			{object}	StatusResponse
//	@Failure		400			{object}	httputil.APIError
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/files [post]
func (s *SandboxRoutes) WriteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	var req orchestrator.FileWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if err := s.manager.WriteFile(r.Context(), id, req); err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "written"})
}

// ReadFile reads a file from a sandbox.
//
//	@Summary		Read a file
//	@Description	Read file content from a sandbox
//	@Tags			sandboxes
//	@Produce		octet-stream
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			path		query		string	true	"File path inside the sandbox"
//	@Success		200			{file}		binary
//	@Failure		400			{object}	httputil.APIError
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/files [get]
func (s *SandboxRoutes) ReadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "path query parameter required")
		return
	}

	data, err := s.manager.ReadFile(r.Context(), id, path)
	if err != nil {
		writeRouteError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// ListFiles lists files in a sandbox directory.
//
//	@Summary		List files
//	@Description	List files in a directory inside a sandbox
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			path		query		string	false	"Directory path (default: /)"
//	@Success		200			{array}		orchestrator.FileInfo
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/files/list [get]
func (s *SandboxRoutes) ListFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	files, err := s.manager.ListFiles(r.Context(), id, path)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, files)
}

// DeleteFile deletes a file from a sandbox.
func (s *SandboxRoutes) DeleteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "path query parameter required")
		return
	}

	recursive := r.URL.Query().Get("recursive") == "true"

	if err := s.manager.DeleteFile(r.Context(), id, orchestrator.FileDeleteRequest{
		Path:      path,
		Recursive: recursive,
	}); err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// MoveFile moves/renames a file in a sandbox.
func (s *SandboxRoutes) MoveFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	var req orchestrator.FileMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if err := s.manager.MoveFile(r.Context(), id, req); err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "moved"})
}

// ChmodFile changes file permissions in a sandbox.
func (s *SandboxRoutes) ChmodFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	var req orchestrator.FileChmodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if err := s.manager.ChmodFile(r.Context(), id, req); err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "chmod applied"})
}

// StatFile returns file info for a single file in a sandbox.
func (s *SandboxRoutes) StatFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "path query parameter required")
		return
	}

	fi, err := s.manager.StatFile(r.Context(), id, path)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, fi)
}

// GlobFiles returns paths matching a glob pattern in a sandbox.
func (s *SandboxRoutes) GlobFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "pattern query parameter required")
		return
	}

	matches, err := s.manager.GlobFiles(r.Context(), id, pattern)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, matches)
}

// PoolStatusResponse is returned by the pool status endpoint.
type PoolStatusResponse = orchestrator.VMPoolStatus

// VMPoolStatusHandler returns the VM pool status.
func (s *SandboxRoutes) VMPoolStatus(w http.ResponseWriter, r *http.Request) {
	status := s.manager.VMPoolStatus()
	if status == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]bool{"enabled": false})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, status)
}

// Prune destroys all expired sandboxes.
//
//	@Summary		Prune sandboxes
//	@Description	Destroy all expired sandboxes and return the count
//	@Tags			sandboxes
//	@Produce		json
//	@Success		200	{object}	PruneResponse
//	@Failure		500	{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes [delete]
func (s *SandboxRoutes) Prune(w http.ResponseWriter, r *http.Request) {
	count, err := s.manager.Prune(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"pruned": count})
}

// ConsoleLog retrieves console log output from a sandbox.
//
//	@Summary		Get console logs
//	@Description	Retrieve console log lines from a sandbox
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			lines		query		int		false	"Number of lines to retrieve (default: 100)"
//	@Success		200			{array}		string
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/logs [get]
func (s *SandboxRoutes) ConsoleLog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}
	lines := 100
	if q := r.URL.Query().Get("lines"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			lines = n
		}
	}

	log, err := s.manager.ConsoleLog(r.Context(), id, lines)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, log)
}

// ExecWebSocket executes a command over WebSocket.
//
//	@Summary		Execute via WebSocket
//	@Description	Open a WebSocket connection to execute a command with streaming output
//	@Tags			sandboxes
//	@Param			sandboxID	path	string	true	"Sandbox ID"
//	@Success		101			"WebSocket upgrade"
//	@Failure		400			"Bad request"
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/exec/ws [get]
func (s *SandboxRoutes) ExecWebSocket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if !s.checkTenantAccess(w, r, id) {
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Read command from client
	var req orchestrator.ExecRequest
	if err := wsjson.Read(ctx, conn, &req); err != nil {
		conn.Close(websocket.StatusInvalidFramePayloadData, "invalid request")
		return
	}

	ch, err := s.manager.ExecStream(ctx, id, req)
	if err != nil {
		wsjson.Write(ctx, conn, map[string]string{"error": err.Error()})
		conn.Close(websocket.StatusInternalError, "exec failed")
		return
	}

	for chunk := range ch {
		if err := wsjson.Write(ctx, conn, chunk); err != nil {
			return
		}
	}

	wsjson.Write(ctx, conn, map[string]string{"type": "done"})
}
