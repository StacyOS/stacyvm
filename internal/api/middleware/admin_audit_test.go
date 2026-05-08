package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

type memoryAuditStore struct {
	records []*store.AdminAuditRecord
	deleted int64
}

func (s *memoryAuditStore) CreateAdminAudit(ctx context.Context, rec *store.AdminAuditRecord) error {
	s.records = append(s.records, rec)
	return nil
}

func (s *memoryAuditStore) DeleteAdminAuditBefore(ctx context.Context, before time.Time) (int64, error) {
	var kept []*store.AdminAuditRecord
	for _, rec := range s.records {
		if rec.CreatedAt.Before(before) {
			s.deleted++
			continue
		}
		kept = append(kept, rec)
	}
	s.records = kept
	return s.deleted, nil
}

func TestAdminAuditPrunesWithRetention(t *testing.T) {
	st := &memoryAuditStore{
		records: []*store.AdminAuditRecord{
			{Path: "/old", CreatedAt: time.Now().Add(-2 * time.Hour)},
		},
	}
	handler := AdminAudit(st, zerolog.Nop(), time.Hour)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if st.deleted != 1 {
		t.Fatalf("deleted = %d, want 1", st.deleted)
	}
	if len(st.records) != 1 || st.records[0].Path != "/api/v1/admin/diagnostics" {
		t.Fatalf("unexpected audit records after prune: %+v", st.records)
	}
}

func TestAdminAuditUsesAuthenticatedRoleWhenActorHeaderMissing(t *testing.T) {
	st := &memoryAuditStore{}
	handler := AdminAudit(st, zerolog.Nop(), 0)(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/quotas", nil)
	req = req.WithContext(WithAuthIdentity(req.Context(), AuthIdentity{Role: AuthRoleAdmin}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(st.records) != 1 {
		t.Fatalf("records = %d, want 1", len(st.records))
	}
	if st.records[0].Actor != "admin" {
		t.Fatalf("actor = %q, want admin", st.records[0].Actor)
	}
}
