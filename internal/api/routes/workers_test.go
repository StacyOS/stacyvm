package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
)

func setupWorkerRoutes(t *testing.T) (*WorkerRoutes, *store.SQLiteStore) {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewWorkerRoutes(st), st
}

func TestWorkerRoutes_HeartbeatListGetDelete(t *testing.T) {
	routes, _ := setupWorkerRoutes(t)
	router := chi.NewRouter()
	router.Mount("/workers", routes.Routes())

	body := []byte(`{
		"hostname":"host-a",
		"status":"online",
		"providers":["mock","docker"],
		"capabilities":["spawn","exec"],
		"capacity":{"max_sandboxes":10}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/workers/worker-a/heartbeat", bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var heartbeat WorkerResponse
	if err := json.NewDecoder(w.Body).Decode(&heartbeat); err != nil {
		t.Fatalf("decode heartbeat: %v", err)
	}
	if heartbeat.ID != "worker-a" || heartbeat.Hostname != "host-a" || heartbeat.Stale {
		t.Fatalf("unexpected heartbeat response: %+v", heartbeat)
	}
	if len(heartbeat.Providers) != 2 || heartbeat.Providers[0] != "mock" {
		t.Fatalf("unexpected providers: %+v", heartbeat.Providers)
	}

	req = httptest.NewRequest(http.MethodGet, "/workers", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var workers []WorkerResponse
	if err := json.NewDecoder(w.Body).Decode(&workers); err != nil {
		t.Fatalf("decode workers: %v", err)
	}
	if len(workers) != 1 || workers[0].ID != "worker-a" {
		t.Fatalf("unexpected workers: %+v", workers)
	}

	req = httptest.NewRequest(http.MethodGet, "/workers/worker-a", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/workers/worker-a", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/workers/worker-a", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestWorkerRoutes_HeartbeatRejectsInvalidJSON(t *testing.T) {
	routes, _ := setupWorkerRoutes(t)
	router := chi.NewRouter()
	router.Mount("/workers", routes.Routes())

	req := httptest.NewRequest(http.MethodPost, "/workers/worker-a/heartbeat", bytes.NewReader([]byte(`{`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
