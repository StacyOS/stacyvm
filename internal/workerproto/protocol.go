// Package workerproto defines the control-plane contract used to assign
// sandbox lifecycle work to StacyVM workers.
//
// This package intentionally defines payloads only. Phase 10 makes the
// contract explicit without choosing a network transport yet.
package workerproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MethodHeartbeat  = "worker.heartbeat"
	MethodSpawn      = "worker.spawn"
	MethodDestroy    = "worker.destroy"
	MethodStatus     = "worker.status"
	MethodExec       = "worker.exec"
	MethodExecStream = "worker.exec_stream"
	MethodRenewLease = "worker.renew_lease"
	MethodShutdown   = "worker.shutdown"
)

const (
	ScopeHeartbeat = "worker:heartbeat"
	ScopeSpawn     = "worker:spawn"
	ScopeDestroy   = "worker:destroy"
	ScopeStatus    = "worker:status"
	ScopeExec      = "worker:exec"
	ScopeLease     = "worker:lease"
)

var (
	ErrInvalidMessage = errors.New("invalid worker message")
	ErrUnknownMethod  = errors.New("unknown worker method")
)

// Request is the control-plane to worker envelope.
type Request struct {
	ID       string          `json:"id"`
	Method   string          `json:"method"`
	WorkerID string          `json:"worker_id"`
	Lease    *LeaseToken     `json:"lease,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
}

// Response is the worker to control-plane envelope.
type Response struct {
	ID       string          `json:"id"`
	WorkerID string          `json:"worker_id"`
	Error    string          `json:"error,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
}

// LeaseToken carries the fencing information a worker must present when
// mutating sandbox lifecycle state.
type LeaseToken struct {
	ResourceID string    `json:"resource_id"`
	HolderID   string    `json:"holder_id"`
	Generation int64     `json:"generation"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// HeartbeatParams are sent by workers to report liveness and capacity.
type HeartbeatParams struct {
	Hostname     string                 `json:"hostname"`
	Status       string                 `json:"status"`
	Providers    []string               `json:"providers"`
	Capabilities []string               `json:"capabilities"`
	Capacity     map[string]interface{} `json:"capacity"`
}

// SpawnParams assigns sandbox creation to a worker.
type SpawnParams struct {
	SandboxID string            `json:"sandbox_id"`
	Image     string            `json:"image"`
	Provider  string            `json:"provider"`
	MemoryMB  int               `json:"memory_mb"`
	VCPUs     int               `json:"vcpus"`
	OwnerID   string            `json:"owner_id,omitempty"`
	TTL       string            `json:"ttl"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SpawnResult is returned once a worker has created a runtime sandbox.
type SpawnResult struct {
	SandboxID string            `json:"sandbox_id"`
	RuntimeID string            `json:"runtime_id,omitempty"`
	State     string            `json:"state"`
	Provider  string            `json:"provider"`
	WorkerID  string            `json:"worker_id"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// DestroyParams assigns sandbox destruction to a worker.
type DestroyParams struct {
	SandboxID string `json:"sandbox_id"`
	Provider  string `json:"provider"`
	RuntimeID string `json:"runtime_id,omitempty"`
}

// StatusParams asks a worker to report runtime state for one sandbox.
type StatusParams struct {
	SandboxID string `json:"sandbox_id"`
	Provider  string `json:"provider"`
	RuntimeID string `json:"runtime_id,omitempty"`
}

// StatusResult reports worker-observed runtime state.
type StatusResult struct {
	SandboxID string `json:"sandbox_id"`
	State     string `json:"state"`
	Provider  string `json:"provider"`
	WorkerID  string `json:"worker_id"`
	Error     string `json:"error,omitempty"`
}

// ExecParams asks a worker to run a non-streaming command in an owned runtime.
type ExecParams struct {
	SandboxID string            `json:"sandbox_id"`
	Provider  string            `json:"provider"`
	RuntimeID string            `json:"runtime_id,omitempty"`
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Mode      string            `json:"mode,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	WorkDir   string            `json:"workdir,omitempty"`
	Timeout   string            `json:"timeout,omitempty"`
}

// ExecResult reports the completed command result from a worker runtime.
type ExecResult struct {
	SandboxID string `json:"sandbox_id"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
}

// StreamChunk is one stdout/stderr payload emitted by worker.exec_stream.
type StreamChunk struct {
	Stream string `json:"stream"`
	Data   string `json:"data"`
}

// ExecStreamResult reports buffered stream chunks from a worker runtime.
type ExecStreamResult struct {
	SandboxID string        `json:"sandbox_id"`
	Chunks    []StreamChunk `json:"chunks"`
}

// RenewLeaseParams asks a worker to confirm it still owns work and needs a
// renewed lease window.
type RenewLeaseParams struct {
	ResourceID string `json:"resource_id"`
	TTL        string `json:"ttl"`
}

// RenewLeaseResult returns the updated fencing token.
type RenewLeaseResult struct {
	Lease LeaseToken `json:"lease"`
}

// AuthClaims are the transport-neutral identity facts a validated worker token
// must produce.
type AuthClaims struct {
	WorkerID string    `json:"worker_id"`
	Scopes   []string  `json:"scopes"`
	Expires  time.Time `json:"expires"`
}

func (c AuthClaims) HasScope(scope string) bool {
	for _, candidate := range c.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func ValidateRequest(req Request) error {
	if strings.TrimSpace(req.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidMessage)
	}
	if strings.TrimSpace(req.WorkerID) == "" {
		return fmt.Errorf("%w: worker_id is required", ErrInvalidMessage)
	}
	switch req.Method {
	case MethodHeartbeat:
		return validateParams[HeartbeatParams](req.Params)
	case MethodSpawn:
		if req.Lease == nil {
			return fmt.Errorf("%w: lease is required for spawn", ErrInvalidMessage)
		}
		return validateParams[SpawnParams](req.Params)
	case MethodDestroy:
		if req.Lease == nil {
			return fmt.Errorf("%w: lease is required for destroy", ErrInvalidMessage)
		}
		return validateParams[DestroyParams](req.Params)
	case MethodStatus:
		return validateParams[StatusParams](req.Params)
	case MethodExec:
		return validateParams[ExecParams](req.Params)
	case MethodExecStream:
		return validateParams[ExecParams](req.Params)
	case MethodRenewLease:
		if req.Lease == nil {
			return fmt.Errorf("%w: lease is required for renew_lease", ErrInvalidMessage)
		}
		return validateParams[RenewLeaseParams](req.Params)
	case MethodShutdown:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnknownMethod, req.Method)
	}
}

func validateParams[T any](raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%w: params are required", ErrInvalidMessage)
	}
	var params T
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("%w: invalid params: %v", ErrInvalidMessage, err)
	}
	return nil
}
