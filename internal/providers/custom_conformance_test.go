package providers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCustomProvider_Conformance(t *testing.T) {
	server := newFakeCustomProviderServer(t)
	defer server.Close()

	runProviderConformance(t, func(t *testing.T) Provider {
		t.Helper()
		return NewCustomProvider(CustomProviderConfig{
			BaseURL: server.URL,
			Timeout: 5 * time.Second,
		})
	})
}

type fakeCustomBackend struct {
	mock *MockProvider
}

func newFakeCustomProviderServer(t *testing.T) *httptest.Server {
	t.Helper()
	backend := &fakeCustomBackend{mock: NewMockProvider()}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", backend.health)
	mux.HandleFunc("/spawn", backend.spawn)
	mux.HandleFunc("/exec", backend.exec)
	mux.HandleFunc("/files", backend.files)
	mux.HandleFunc("/files/list", backend.listFiles)
	mux.HandleFunc("/files/move", backend.moveFile)
	mux.HandleFunc("/files/chmod", backend.chmodFile)
	mux.HandleFunc("/files/stat", backend.statFile)
	mux.HandleFunc("/files/glob", backend.globFiles)
	mux.HandleFunc("/status/", backend.status)
	mux.HandleFunc("/sandboxes/", backend.destroy)
	return httptest.NewServer(mux)
}

func (b *fakeCustomBackend) health(w http.ResponseWriter, r *http.Request) {
	writeFakeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (b *fakeCustomBackend) spawn(w http.ResponseWriter, r *http.Request) {
	id, err := b.mock.Spawn(r.Context(), SpawnOptions{})
	if err != nil {
		writeFakeError(w, err)
		return
	}
	writeFakeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (b *fakeCustomBackend) exec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SandboxID string `json:"sandbox_id"`
		Command   string `json:"command"`
		Stream    bool   `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Stream {
		ch, err := b.mock.ExecStream(r.Context(), req.SandboxID, ExecOptions{Command: req.Command})
		if err != nil {
			writeFakeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		for chunk := range ch {
			_ = enc.Encode(chunk)
		}
		return
	}
	result, err := b.mock.Exec(r.Context(), req.SandboxID, ExecOptions{Command: req.Command})
	if err != nil {
		writeFakeError(w, err)
		return
	}
	writeFakeJSON(w, http.StatusOK, map[string]any{
		"exit_code": result.ExitCode,
		"stdout":    result.Stdout,
		"stderr":    result.Stderr,
	})
}

func (b *fakeCustomBackend) files(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rc, err := b.mock.ReadFile(r.Context(), r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("path"))
		if err != nil {
			writeFakeError(w, err)
			return
		}
		defer rc.Close()
		_, _ = io.Copy(w, rc)
	case http.MethodPost:
		var req struct {
			SandboxID string `json:"sandbox_id"`
			Path      string `json:"path"`
			Content   string `json:"content"`
			Mode      string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := b.mock.WriteFile(r.Context(), req.SandboxID, req.Path, strings.NewReader(req.Content), req.Mode); err != nil {
			writeFakeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		var req struct {
			SandboxID string `json:"sandbox_id"`
			Path      string `json:"path"`
			Recursive bool   `json:"recursive"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := b.mock.DeleteFile(r.Context(), req.SandboxID, req.Path, req.Recursive); err != nil {
			writeFakeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (b *fakeCustomBackend) listFiles(w http.ResponseWriter, r *http.Request) {
	files, err := b.mock.ListFiles(r.Context(), r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("path"))
	if err != nil {
		writeFakeError(w, err)
		return
	}
	writeFakeJSON(w, http.StatusOK, files)
}

func (b *fakeCustomBackend) moveFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SandboxID string `json:"sandbox_id"`
		OldPath   string `json:"old_path"`
		NewPath   string `json:"new_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := b.mock.MoveFile(r.Context(), req.SandboxID, req.OldPath, req.NewPath); err != nil {
		writeFakeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *fakeCustomBackend) chmodFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SandboxID string `json:"sandbox_id"`
		Path      string `json:"path"`
		Mode      string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := b.mock.ChmodFile(r.Context(), req.SandboxID, req.Path, req.Mode); err != nil {
		writeFakeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *fakeCustomBackend) statFile(w http.ResponseWriter, r *http.Request) {
	fi, err := b.mock.StatFile(r.Context(), r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("path"))
	if err != nil {
		writeFakeError(w, err)
		return
	}
	writeFakeJSON(w, http.StatusOK, fi)
}

func (b *fakeCustomBackend) globFiles(w http.ResponseWriter, r *http.Request) {
	matches, err := b.mock.GlobFiles(r.Context(), r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("pattern"))
	if err != nil {
		writeFakeError(w, err)
		return
	}
	writeFakeJSON(w, http.StatusOK, matches)
}

func (b *fakeCustomBackend) status(w http.ResponseWriter, r *http.Request) {
	id := path.Base(r.URL.Path)
	status, err := b.mock.Status(r.Context(), id)
	if err != nil {
		writeFakeError(w, err)
		return
	}
	writeFakeJSON(w, http.StatusOK, status)
}

func (b *fakeCustomBackend) destroy(w http.ResponseWriter, r *http.Request) {
	id := path.Base(r.URL.Path)
	if err := b.mock.Destroy(r.Context(), id); err != nil {
		writeFakeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeFakeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeFakeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, ErrSandboxNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, ErrSandboxDestroyed) {
		status = http.StatusGone
	}
	http.Error(w, strconv.Quote(err.Error()), status)
}
