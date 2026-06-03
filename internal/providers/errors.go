package providers

import (
	"errors"
	"fmt"
)

var (
	ErrSandboxNotFound     = errors.New("sandbox not found")
	ErrSandboxDestroyed    = errors.New("sandbox destroyed")
	ErrProviderNotFound    = errors.New("provider not found")
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrExecTimeout         = errors.New("exec timeout")
	ErrResourceLimit       = errors.New("resource limit exceeded")
	ErrPTYUnsupported      = errors.New("interactive pty not supported by provider")
)

func SandboxNotFoundError(id string) error {
	return fmt.Errorf("%w: %s", ErrSandboxNotFound, id)
}

func SandboxDestroyedError(id string) error {
	return fmt.Errorf("%w: %s", ErrSandboxDestroyed, id)
}

func ProviderNotFoundError(name string) error {
	return fmt.Errorf("%w: %s", ErrProviderNotFound, name)
}

func ProviderUnavailableError(name string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrProviderUnavailable, name)
	}
	return fmt.Errorf("%w: %s: %v", ErrProviderUnavailable, name, err)
}

func ExecTimeoutError(sandboxID string) error {
	return fmt.Errorf("%w: %s", ErrExecTimeout, sandboxID)
}

func ResourceLimitError(resource string) error {
	return fmt.Errorf("%w: %s", ErrResourceLimit, resource)
}
