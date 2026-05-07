package orchestrator

import "github.com/StacyOs/stacyvm/internal/providers"

var (
	ErrSandboxNotFound     = providers.ErrSandboxNotFound
	ErrSandboxDestroyed    = providers.ErrSandboxDestroyed
	ErrProviderNotFound    = providers.ErrProviderNotFound
	ErrProviderUnavailable = providers.ErrProviderUnavailable
	ErrExecTimeout         = providers.ErrExecTimeout
	ErrResourceLimit       = providers.ErrResourceLimit
)
