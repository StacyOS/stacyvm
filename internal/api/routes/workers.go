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
)

type workerStore interface {
	SaveWorker(ctx context.Context, rec *store.WorkerRecord) error
	GetWorker(ctx context.Context, id string) (*store.WorkerRecord, error)
	ListWorkers(ctx context.Context) ([]*store.WorkerRecord, error)
	DeleteWorker(ctx context.Context, id string) error
}

type WorkerRoutes struct {
	store workerStore
}

func NewWorkerRoutes(st workerStore) *WorkerRoutes {
	return &WorkerRoutes{store: st}
}

func (w *WorkerRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", w.List)
	r.Get("/{workerID}", w.Get)
	r.Post("/{workerID}/heartbeat", w.Heartbeat)
	r.Delete("/{workerID}", w.Delete)
	return r
}

func (w *WorkerRoutes) ReadOnlyRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", w.List)
	r.Get("/{workerID}", w.Get)
	return r
}

type WorkerHeartbeatRequest struct {
	Hostname     string                 `json:"hostname" example:"stacyvm-host-1"`
	Status       string                 `json:"status" example:"online"`
	Providers    []string               `json:"providers"`
	Capabilities []string               `json:"capabilities"`
	Capacity     map[string]interface{} `json:"capacity"`
}

type WorkerResponse struct {
	ID            string                 `json:"id" example:"worker-local"`
	Hostname      string                 `json:"hostname" example:"stacyvm-host-1"`
	Status        string                 `json:"status" example:"online"`
	Providers     []string               `json:"providers"`
	Capabilities  []string               `json:"capabilities"`
	Capacity      map[string]interface{} `json:"capacity"`
	LastHeartbeat string                 `json:"last_heartbeat" example:"2026-05-09T10:30:00Z"`
	CreatedAt     string                 `json:"created_at" example:"2026-05-09T10:00:00Z"`
	UpdatedAt     string                 `json:"updated_at" example:"2026-05-09T10:30:00Z"`
	Stale         bool                   `json:"stale" example:"false"`
}

// List returns all registered workers.
//
//	@Summary		List workers
//	@Description	Return worker registry records and heartbeat state
//	@Tags			workers
//	@Produce		json
//	@Success		200	{array}		WorkerResponse
//	@Security		ApiKeyAuth
//	@Router			/workers [get]
func (w *WorkerRoutes) List(rw http.ResponseWriter, r *http.Request) {
	if w.store == nil {
		httputil.WriteJSON(rw, http.StatusOK, []WorkerResponse{})
		return
	}
	records, err := w.store.ListWorkers(r.Context())
	if err != nil {
		writeRouteError(rw, err)
		return
	}
	responses := make([]WorkerResponse, 0, len(records))
	for _, rec := range records {
		responses = append(responses, workerResponse(rec, time.Now().UTC()))
	}
	httputil.WriteJSON(rw, http.StatusOK, responses)
}

// Get returns one registered worker.
//
//	@Summary		Get worker
//	@Description	Return one worker registry record
//	@Tags			workers
//	@Produce		json
//	@Param			workerID	path		string	true	"Worker ID"
//	@Success		200			{object}	WorkerResponse
//	@Failure		404			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/workers/{workerID} [get]
func (w *WorkerRoutes) Get(rw http.ResponseWriter, r *http.Request) {
	rec, err := w.store.GetWorker(r.Context(), chi.URLParam(r, "workerID"))
	if err != nil {
		writeRouteError(rw, err)
		return
	}
	httputil.WriteJSON(rw, http.StatusOK, workerResponse(rec, time.Now().UTC()))
}

// Heartbeat creates or updates a worker heartbeat.
//
//	@Summary		Heartbeat worker
//	@Description	Create or update worker registry state for a worker
//	@Tags			workers
//	@Accept			json
//	@Produce		json
//	@Param			workerID	path		string					true	"Worker ID"
//	@Param			request		body		WorkerHeartbeatRequest	true	"Worker heartbeat"
//	@Success		200			{object}	WorkerResponse
//	@Security		ApiKeyAuth
//	@Router			/workers/{workerID}/heartbeat [post]
func (w *WorkerRoutes) Heartbeat(rw http.ResponseWriter, r *http.Request) {
	if w.store == nil {
		httputil.WriteError(rw, http.StatusServiceUnavailable, httputil.CodeUnavailable, "worker store unavailable")
		return
	}
	workerID := strings.TrimSpace(chi.URLParam(r, "workerID"))
	if workerID == "" {
		httputil.WriteError(rw, http.StatusBadRequest, httputil.CodeBadRequest, "worker id is required")
		return
	}
	var req WorkerHeartbeatRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(rw, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
			return
		}
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "online"
	}
	now := time.Now().UTC()
	rec := &store.WorkerRecord{
		ID:            workerID,
		Hostname:      strings.TrimSpace(req.Hostname),
		Status:        status,
		Providers:     mustJSON(req.Providers, []string{}),
		Capabilities:  mustJSON(req.Capabilities, []string{}),
		Capacity:      mustJSON(req.Capacity, map[string]interface{}{}),
		LastHeartbeat: now,
	}
	if existing, err := w.store.GetWorker(r.Context(), workerID); err == nil {
		rec.CreatedAt = existing.CreatedAt
	} else if !errors.Is(err, store.ErrNotFound) {
		writeRouteError(rw, err)
		return
	}
	if err := w.store.SaveWorker(r.Context(), rec); err != nil {
		writeRouteError(rw, err)
		return
	}
	httputil.WriteJSON(rw, http.StatusOK, workerResponse(rec, now))
}

// Delete removes one worker registry record.
//
//	@Summary		Delete worker
//	@Description	Remove a worker registry record
//	@Tags			workers
//	@Param			workerID	path	string	true	"Worker ID"
//	@Success		200			{object}	StatusResponse
//	@Failure		404			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/workers/{workerID} [delete]
func (w *WorkerRoutes) Delete(rw http.ResponseWriter, r *http.Request) {
	if err := w.store.DeleteWorker(r.Context(), chi.URLParam(r, "workerID")); err != nil {
		writeRouteError(rw, err)
		return
	}
	httputil.WriteJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

func workerResponse(rec *store.WorkerRecord, now time.Time) WorkerResponse {
	var providers []string
	var capabilities []string
	capacity := map[string]interface{}{}
	_ = json.Unmarshal([]byte(rec.Providers), &providers)
	_ = json.Unmarshal([]byte(rec.Capabilities), &capabilities)
	_ = json.Unmarshal([]byte(rec.Capacity), &capacity)
	return WorkerResponse{
		ID:            rec.ID,
		Hostname:      rec.Hostname,
		Status:        rec.Status,
		Providers:     providers,
		Capabilities:  capabilities,
		Capacity:      capacity,
		LastHeartbeat: rec.LastHeartbeat.UTC().Format(time.RFC3339),
		CreatedAt:     rec.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     rec.UpdatedAt.UTC().Format(time.RFC3339),
		Stale:         now.Sub(rec.LastHeartbeat) > 2*time.Minute,
	}
}

func mustJSON(value interface{}, fallback interface{}) string {
	if value == nil {
		value = fallback
	}
	data, err := json.Marshal(value)
	if err != nil {
		data, _ = json.Marshal(fallback)
	}
	return string(data)
}
