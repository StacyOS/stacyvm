package routes

import (
	"errors"
	"net/http"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/store"
)

func writeRouteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, orchestrator.ErrInvalidInput):
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, err.Error())
	case errors.Is(err, orchestrator.ErrSandboxNotFound),
		errors.Is(err, orchestrator.ErrSandboxDestroyed),
		errors.Is(err, orchestrator.ErrProviderNotFound),
		errors.Is(err, store.ErrNotFound):
		httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
	case errors.Is(err, store.ErrConflict):
		httputil.WriteError(w, http.StatusConflict, httputil.CodeConflict, err.Error())
	case errors.Is(err, orchestrator.ErrExecTimeout):
		httputil.WriteError(w, http.StatusRequestTimeout, httputil.CodeTimeout, err.Error())
	case errors.Is(err, orchestrator.ErrResourceLimit):
		httputil.WriteError(w, http.StatusTooManyRequests, httputil.CodeResourceLimit, err.Error())
	case errors.Is(err, orchestrator.ErrProviderUnavailable):
		httputil.WriteError(w, http.StatusServiceUnavailable, httputil.CodeUnavailable, err.Error())
	default:
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
	}
}
