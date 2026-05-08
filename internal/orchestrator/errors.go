package orchestrator

import (
	"errors"
	"fmt"

	"github.com/StacyOs/stacyvm/internal/providers"
)

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrSandboxNotFound     = providers.ErrSandboxNotFound
	ErrSandboxDestroyed    = providers.ErrSandboxDestroyed
	ErrProviderNotFound    = providers.ErrProviderNotFound
	ErrProviderUnavailable = providers.ErrProviderUnavailable
	ErrExecTimeout         = providers.ErrExecTimeout
	ErrResourceLimit       = providers.ErrResourceLimit
)

func InvalidInputError(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, message)
}
