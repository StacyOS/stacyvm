package routes

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/go-chi/chi/v5"
)

type quotaManager interface {
	ListOwnerQuotas(ctx context.Context) ([]*orchestrator.OwnerQuota, error)
	GetOwnerQuota(ctx context.Context, ownerID string) (*orchestrator.OwnerQuota, error)
	SaveOwnerQuota(ctx context.Context, quota orchestrator.OwnerQuota) (*orchestrator.OwnerQuota, error)
	DeleteOwnerQuota(ctx context.Context, ownerID string) error
	OwnerUsage(ctx context.Context, ownerID string) (*orchestrator.OwnerUsage, error)
}

type QuotaRoutes struct {
	manager quotaManager
}

func NewQuotaRoutes(manager quotaManager) *QuotaRoutes {
	return &QuotaRoutes{manager: manager}
}

func (q *QuotaRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", q.List)
	r.Route("/{ownerID}", func(r chi.Router) {
		r.Get("/", q.Get)
		r.Put("/", q.Save)
		r.Delete("/", q.Delete)
		r.Get("/usage", q.Usage)
	})
	return r
}

// List returns all configured owner quotas.
//
//	@Summary		List owner quotas
//	@Description	Return all persisted owner quota overrides
//	@Tags			quotas
//	@Produce		json
//	@Success		200	{array}		orchestrator.OwnerQuota
//	@Security		ApiKeyAuth
//	@Router			/quotas [get]
func (q *QuotaRoutes) List(w http.ResponseWriter, r *http.Request) {
	quotas, err := q.manager.ListOwnerQuotas(r.Context())
	if err != nil {
		writeRouteError(w, err)
		return
	}
	if quotas == nil {
		quotas = []*orchestrator.OwnerQuota{}
	}
	httputil.WriteJSON(w, http.StatusOK, quotas)
}

// Get returns one configured owner quota.
//
//	@Summary		Get owner quota
//	@Description	Return the persisted quota override for an owner
//	@Tags			quotas
//	@Produce		json
//	@Param			ownerID	path		string	true	"Owner ID"
//	@Success		200		{object}	orchestrator.OwnerQuota
//	@Failure		404		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/quotas/{ownerID} [get]
func (q *QuotaRoutes) Get(w http.ResponseWriter, r *http.Request) {
	quota, err := q.manager.GetOwnerQuota(r.Context(), chi.URLParam(r, "ownerID"))
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, quota)
}

// Save creates or updates an owner quota.
//
//	@Summary		Save owner quota
//	@Description	Create or update quota overrides for an owner
//	@Tags			quotas
//	@Accept			json
//	@Produce		json
//	@Param			ownerID	path		string					true	"Owner ID"
//	@Param			request	body		orchestrator.OwnerQuota	true	"Quota request"
//	@Success		200		{object}	orchestrator.OwnerQuota
//	@Security		ApiKeyAuth
//	@Router			/quotas/{ownerID} [put]
func (q *QuotaRoutes) Save(w http.ResponseWriter, r *http.Request) {
	ownerID := chi.URLParam(r, "ownerID")
	var quota orchestrator.OwnerQuota
	if err := json.NewDecoder(r.Body).Decode(&quota); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	quota.OwnerID = ownerID
	saved, err := q.manager.SaveOwnerQuota(r.Context(), quota)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
}

// Delete removes an owner quota override.
//
//	@Summary		Delete owner quota
//	@Description	Delete the quota override for an owner
//	@Tags			quotas
//	@Produce		json
//	@Param			ownerID	path		string	true	"Owner ID"
//	@Success		200		{object}	StatusResponse
//	@Failure		404		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/quotas/{ownerID} [delete]
func (q *QuotaRoutes) Delete(w http.ResponseWriter, r *http.Request) {
	if err := q.manager.DeleteOwnerQuota(r.Context(), chi.URLParam(r, "ownerID")); err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Usage returns current owner usage against quota.
//
//	@Summary		Get owner quota usage
//	@Description	Return active sandbox usage and effective quota for an owner
//	@Tags			quotas
//	@Produce		json
//	@Param			ownerID	path		string	true	"Owner ID"
//	@Success		200		{object}	orchestrator.OwnerUsage
//	@Security		ApiKeyAuth
//	@Router			/quotas/{ownerID}/usage [get]
func (q *QuotaRoutes) Usage(w http.ResponseWriter, r *http.Request) {
	usage, err := q.manager.OwnerUsage(r.Context(), chi.URLParam(r, "ownerID"))
	if err != nil {
		writeRouteError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, usage)
}
