package routes

import (
	"context"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/store"
)

type adminAuditStore interface {
	ListAdminAudit(ctx context.Context, query store.AdminAuditQuery) ([]*store.AdminAuditRecord, error)
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
//	@Param			actor	query		string	false	"Actor exact match"
//	@Param			method	query		string	false	"HTTP method exact match"
//	@Param			status	query		int	false	"HTTP status exact match"
//	@Param			path	query		string	false	"Path substring match"
//	@Param			format	query		string	false	"Response format: json or csv"
//	@Success		200		{array}		AdminAuditResponse
//	@Security		ApiKeyAuth
//	@Router			/admin/audit [get]
func (a *AdminAuditRoutes) List(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		httputil.WriteJSON(w, http.StatusOK, []*store.AdminAuditRecord{})
		return
	}
	query := store.AdminAuditQuery{Limit: 100}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "limit must be a positive integer")
			return
		}
		query.Limit = parsed
	}
	query.Actor = strings.TrimSpace(r.URL.Query().Get("actor"))
	query.Method = strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("method")))
	query.PathLike = strings.TrimSpace(r.URL.Query().Get("path"))
	if raw := r.URL.Query().Get("status"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "status must be a positive integer")
			return
		}
		query.Status = parsed
	}
	records, err := a.store.ListAdminAudit(r.Context(), query)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	if records == nil {
		records = []*store.AdminAuditRecord{}
	}
	if strings.EqualFold(r.URL.Query().Get("format"), "csv") {
		writeAdminAuditCSV(w, records)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, records)
}

func writeAdminAuditCSV(w http.ResponseWriter, records []*store.AdminAuditRecord) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="stacyvm-admin-audit.csv"`)
	w.WriteHeader(http.StatusOK)

	writer := csv.NewWriter(w)
	_ = writer.Write([]string{
		"id",
		"created_at",
		"actor",
		"method",
		"path",
		"status",
		"duration_ms",
		"request_id",
		"remote_addr",
		"user_agent",
	})
	for _, rec := range records {
		_ = writer.Write([]string{
			strconv.FormatInt(rec.ID, 10),
			rec.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			rec.Actor,
			rec.Method,
			rec.Path,
			strconv.Itoa(rec.Status),
			strconv.FormatInt(rec.DurationMS, 10),
			rec.RequestID,
			rec.RemoteAddr,
			rec.UserAgent,
		})
	}
	writer.Flush()
}
