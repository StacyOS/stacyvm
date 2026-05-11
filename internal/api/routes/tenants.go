package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type tenantStore interface {
	CreateTenant(ctx context.Context, t *store.TenantRecord) error
	GetTenant(ctx context.Context, id string) (*store.TenantRecord, error)
	ListTenants(ctx context.Context) ([]*store.TenantRecord, error)
	UpdateTenant(ctx context.Context, t *store.TenantRecord) error
	DeleteTenant(ctx context.Context, id string) error
	SaveTenantMember(ctx context.Context, m *store.TenantMemberRecord) error
	GetTenantMember(ctx context.Context, tenantID, userID string) (*store.TenantMemberRecord, error)
	ListTenantMembers(ctx context.Context, tenantID string) ([]*store.TenantMemberRecord, error)
	DeleteTenantMember(ctx context.Context, tenantID, userID string) error
	ListAdminAudit(ctx context.Context, query store.AdminAuditQuery) ([]*store.AdminAuditRecord, error)
	ListOperationAudit(ctx context.Context, query store.OperationAuditQuery) ([]*store.OperationAuditRecord, error)
	CreatePolicy(ctx context.Context, p *store.PolicyRecord) error
	GetPolicy(ctx context.Context, id string) (*store.PolicyRecord, error)
	ListPolicies(ctx context.Context, query store.PolicyQuery) ([]*store.PolicyRecord, error)
	DeletePolicy(ctx context.Context, id string) error
}

type TenantRoutes struct {
	store tenantStore
}

func NewTenantRoutes(st tenantStore) *TenantRoutes {
	return &TenantRoutes{store: st}
}

func (tr *TenantRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", tr.List)
	r.Post("/", tr.Create)
	r.Get("/{tenantID}", tr.Get)
	r.Put("/{tenantID}", tr.Update)
	r.Delete("/{tenantID}", tr.Delete)
	r.Get("/{tenantID}/members", tr.ListMembers)
	r.Put("/{tenantID}/members/{userID}", tr.UpsertMember)
	r.Delete("/{tenantID}/members/{userID}", tr.DeleteMember)
	r.Get("/{tenantID}/audit", tr.AuditExport)
	r.Get("/{tenantID}/policies", tr.ListPolicies)
	r.Post("/{tenantID}/policies", tr.CreatePolicy)
	r.Delete("/{tenantID}/policies/{policyID}", tr.DeletePolicy)
	return r
}

type CreateTenantRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	OwnerID  string `json:"owner_id"`
	Settings any    `json:"settings"`
}

type UpsertMemberRequest struct {
	Role string `json:"role"` // viewer, operator, admin
}

type CreatePolicyRequest struct {
	ResourceType string `json:"resource_type"`
	Effect       string `json:"effect"`
	Pattern      string `json:"pattern"`
	Priority     int    `json:"priority"`
}

func (tr *TenantRoutes) List(w http.ResponseWriter, r *http.Request) {
	tenants, err := tr.store.ListTenants(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if tenants == nil {
		tenants = []*store.TenantRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"tenants": tenants})
}

func (tr *TenantRoutes) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "name is required")
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = "tenant-" + uuid.New().String()[:8]
	}
	settings := "{}"
	if req.Settings != nil {
		if b, err := json.Marshal(req.Settings); err == nil {
			settings = string(b)
		}
	}
	t := &store.TenantRecord{
		ID:       id,
		Name:     req.Name,
		OwnerID:  strings.TrimSpace(req.OwnerID),
		Settings: settings,
	}
	if err := tr.store.CreateTenant(r.Context(), t); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"tenant": t})
}

func (tr *TenantRoutes) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	t, err := tr.store.GetTenant(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "tenant not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"tenant": t})
}

func (tr *TenantRoutes) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	existing, err := tr.store.GetTenant(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "tenant not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	var req CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.OwnerID != "" {
		existing.OwnerID = req.OwnerID
	}
	if req.Settings != nil {
		if b, err := json.Marshal(req.Settings); err == nil {
			existing.Settings = string(b)
		}
	}
	if err := tr.store.UpdateTenant(r.Context(), existing); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"tenant": existing})
}

func (tr *TenantRoutes) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	if err := tr.store.DeleteTenant(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "tenant not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (tr *TenantRoutes) ListMembers(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	members, err := tr.store.ListTenantMembers(r.Context(), tenantID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if members == nil {
		members = []*store.TenantMemberRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (tr *TenantRoutes) UpsertMember(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	userID := chi.URLParam(r, "userID")
	var req UpsertMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	role := strings.TrimSpace(req.Role)
	if !isValidMemberRole(role) {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "role must be viewer, operator, or admin")
		return
	}
	m := &store.TenantMemberRecord{TenantID: tenantID, UserID: userID, Role: role}
	if err := tr.store.SaveTenantMember(r.Context(), m); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"member": m})
}

func (tr *TenantRoutes) DeleteMember(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	userID := chi.URLParam(r, "userID")
	if err := tr.store.DeleteTenantMember(r.Context(), tenantID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "member not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (tr *TenantRoutes) AuditExport(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	q := r.URL.Query()
	limit := 200
	since := q.Get("since")

	adminLogs, err := tr.store.ListAdminAudit(r.Context(), store.AdminAuditQuery{
		Limit:    limit,
		TenantID: tenantID,
		PathLike: since, // repurposed: caller can filter by path prefix
	})
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	opLogs, err := tr.store.ListOperationAudit(r.Context(), store.OperationAuditQuery{
		Limit:    limit,
		TenantID: tenantID,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if adminLogs == nil {
		adminLogs = []*store.AdminAuditRecord{}
	}
	if opLogs == nil {
		opLogs = []*store.OperationAuditRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"tenant_id":   tenantID,
		"admin_audit": adminLogs,
		"op_audit":    opLogs,
		"exported_at": time.Now().UTC(),
	})
}

func (tr *TenantRoutes) ListPolicies(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	policies, err := tr.store.ListPolicies(r.Context(), store.PolicyQuery{TenantID: tenantID})
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if policies == nil {
		policies = []*store.PolicyRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"policies": policies})
}

func (tr *TenantRoutes) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	var req CreatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	if !isValidPolicyResourceType(req.ResourceType) {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "resource_type must be image, provider, or network")
		return
	}
	if !isValidPolicyEffect(req.Effect) {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "effect must be allow or deny")
		return
	}
	if strings.TrimSpace(req.Pattern) == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "pattern is required")
		return
	}
	priority := req.Priority
	if priority == 0 {
		priority = 10
	}
	p := &store.PolicyRecord{
		ID:           "pol-" + uuid.New().String()[:8],
		TenantID:     tenantID,
		ResourceType: req.ResourceType,
		Effect:       req.Effect,
		Pattern:      req.Pattern,
		Priority:     priority,
	}
	if err := tr.store.CreatePolicy(r.Context(), p); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"policy": p})
}

func (tr *TenantRoutes) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "policyID")
	if err := tr.store.DeletePolicy(r.Context(), policyID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "policy not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isValidMemberRole(role string) bool {
	switch role {
	case "viewer", "operator", "admin":
		return true
	}
	return false
}

func isValidPolicyResourceType(rt string) bool {
	switch rt {
	case "image", "provider", "network":
		return true
	}
	return false
}

func isValidPolicyEffect(effect string) bool {
	return effect == "allow" || effect == "deny"
}
