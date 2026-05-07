package routes

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

func setupTemplateTestRouter(t *testing.T) chi.Router {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	reg := providers.NewRegistry()
	mock := providers.NewMockProvider()
	reg.Register(mock)
	if err := reg.SetDefault("mock"); err != nil {
		t.Fatalf("set default provider: %v", err)
	}
	events := orchestrator.NewEventBus()
	mgr := orchestrator.NewManager(reg, st, events, zerolog.Nop(), orchestrator.ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
	mgr.Start()
	t.Cleanup(func() { mgr.Stop() })

	r := chi.NewRouter()
	r.Mount("/api/v1/templates", NewTemplateRoutes(orchestrator.NewTemplateRegistry(st), mgr).Routes())
	return r
}

func TestTemplateDuplicateReturnsConflict(t *testing.T) {
	r := setupTemplateTestRouter(t)
	body := `{"name":"node","image":"node:20","ttl_seconds":300}`

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/templates", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if i == 0 && w.Code != http.StatusCreated {
			t.Fatalf("first create: expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if i == 1 && w.Code != http.StatusConflict {
			t.Fatalf("second create: expected 409, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestTemplateMissingReturnsNotFound(t *testing.T) {
	r := setupTemplateTestRouter(t)

	req := httptest.NewRequest("GET", "/api/v1/templates/missing-template", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
