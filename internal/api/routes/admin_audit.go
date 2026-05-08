package routes

import (
	"context"
	"net/http"
	"strconv"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/store"
)

type adminAuditStore interface {
	ListAdminAudit(ctx context.Context, limit int) ([]*store.AdminAuditRecord, error)
}

type AdminAuditRoutes struct {
	store adminAuditStore
}

func NewAdminAuditRoutes(st adminAuditStore) *AdminAuditRoutes {
	return &AdminAuditRoutes{store: st}
}

// List returns recent admin audit log entries.
//
//	@Summary		List admin audit logs
//	@Description	Return recent redacted admin route access records
//	@Tags			admin
//	@Produce		json
//	@Param			limit	query		int	false	"Maximum number of records, capped at 500"
//	@Success		200		{array}		AdminAuditResponse
//	@Security		ApiKeyAuth
//	@Router			/admin/audit [get]
func (a *AdminAuditRoutes) List(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		httputil.WriteJSON(w, http.StatusOK, []*store.AdminAuditRecord{})
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
	}
	records, err := a.store.ListAdminAudit(r.Context(), limit)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	if records == nil {
		records = []*store.AdminAuditRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, records)
}
