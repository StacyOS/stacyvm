package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

type adminAuditStore interface {
	CreateAdminAudit(ctx context.Context, rec *store.AdminAuditRecord) error
	DeleteAdminAuditBefore(ctx context.Context, before time.Time) (int64, error)
}

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *auditResponseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func AdminAudit(st adminAuditStore, logger zerolog.Logger, retention time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if st == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			rw := &auditResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			identity := AuthIdentityFromContext(r.Context())
			rec := &store.AdminAuditRecord{
				Actor:      actorFromRequest(r),
				Method:     r.Method,
				Path:       r.URL.Path,
				Status:     rw.status,
				DurationMS: time.Since(start).Milliseconds(),
				RequestID:  GetRequestID(r.Context()),
				RemoteAddr: clientAddr(r),
				UserAgent:  r.UserAgent(),
				TenantID:   identity.TenantID,
				CreatedAt:  time.Now().UTC(),
			}
			if err := st.CreateAdminAudit(r.Context(), rec); err != nil {
				logger.Warn().Err(err).Str("path", r.URL.Path).Msg("failed to write admin audit log")
				return
			}
			if retention > 0 {
				before := rec.CreatedAt.Add(-retention)
				deleted, err := st.DeleteAdminAuditBefore(r.Context(), before)
				if err != nil {
					logger.Warn().Err(err).Msg("failed to prune admin audit logs")
				} else if deleted > 0 {
					logger.Debug().Int64("deleted", deleted).Dur("retention", retention).Msg("pruned admin audit logs")
				}
			}
		})
	}
}

func actorFromRequest(r *http.Request) string {
	if actor := strings.TrimSpace(r.Header.Get("X-User-ID")); actor != "" {
		return actor
	}
	identity := AuthIdentityFromContext(r.Context())
	if identity.Email != "" {
		return identity.Email
	}
	if identity.Subject != "" {
		return identity.Subject
	}
	if identity.Role != AuthRoleAnonymous && identity.Header != "" {
		return string(identity.Role) + ":" + identity.Header
	} else if identity.Role != AuthRoleAnonymous {
		return string(identity.Role)
	}
	return "admin"
}

func clientAddr(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
