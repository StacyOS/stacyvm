package orchestrator

import (
	"sort"
	"sync"
	"time"
)

const (
	OperationSpawn      = "spawn"
	OperationExec       = "exec"
	OperationExecStream = "exec_stream"
	OperationDestroy    = "destroy"
	OperationFileWrite  = "file_write"
	OperationFileRead   = "file_read"
	OperationFileList   = "file_list"
	OperationFileDelete = "file_delete"
	OperationFileMove   = "file_move"
	OperationFileChmod  = "file_chmod"
	OperationFileStat   = "file_stat"
	OperationFileGlob   = "file_glob"
)

type OperationMetrics struct {
	Operation        string `json:"operation"`
	Provider         string `json:"provider"`
	SuccessTotal     uint64 `json:"success_total"`
	FailureTotal     uint64 `json:"failure_total"`
	LatencyCount     uint64 `json:"latency_count"`
	LatencyTotalMS   uint64 `json:"latency_total_ms"`
	LatencyMinMS     uint64 `json:"latency_min_ms"`
	LatencyMaxMS     uint64 `json:"latency_max_ms"`
	LatencyAvgMS     uint64 `json:"latency_avg_ms"`
	LastError        string `json:"last_error,omitempty"`
	LastObservedUnix int64  `json:"last_observed_unix,omitempty"`
}

type operationMetricKey struct {
	operation string
	provider  string
}

type operationMetricBucket struct {
	successTotal     uint64
	failureTotal     uint64
	latencyCount     uint64
	latencyTotalMS   uint64
	latencyMinMS     uint64
	latencyMaxMS     uint64
	lastError        string
	lastObservedUnix int64
}

type MetricsRecorder struct {
	mu         sync.RWMutex
	operations map[operationMetricKey]*operationMetricBucket
}

func NewMetricsRecorder() *MetricsRecorder {
	return &MetricsRecorder{operations: make(map[operationMetricKey]*operationMetricBucket)}
}

func (r *MetricsRecorder) RecordOperation(operation, provider string, duration time.Duration, err error) {
	if r == nil {
		return
	}
	if provider == "" {
		provider = "unknown"
	}
	latencyMS := uint64(duration.Milliseconds())
	key := operationMetricKey{operation: operation, provider: provider}

	r.mu.Lock()
	defer r.mu.Unlock()

	bucket := r.operations[key]
	if bucket == nil {
		bucket = &operationMetricBucket{latencyMinMS: latencyMS}
		r.operations[key] = bucket
	}
	if err != nil {
		bucket.failureTotal++
		bucket.lastError = err.Error()
	} else {
		bucket.successTotal++
	}
	bucket.latencyCount++
	bucket.latencyTotalMS += latencyMS
	if latencyMS < bucket.latencyMinMS {
		bucket.latencyMinMS = latencyMS
	}
	if latencyMS > bucket.latencyMaxMS {
		bucket.latencyMaxMS = latencyMS
	}
	bucket.lastObservedUnix = time.Now().Unix()
}

func (r *MetricsRecorder) Snapshot() []OperationMetrics {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]OperationMetrics, 0, len(r.operations))
	for key, bucket := range r.operations {
		avg := uint64(0)
		if bucket.latencyCount > 0 {
			avg = bucket.latencyTotalMS / bucket.latencyCount
		}
		out = append(out, OperationMetrics{
			Operation:        key.operation,
			Provider:         key.provider,
			SuccessTotal:     bucket.successTotal,
			FailureTotal:     bucket.failureTotal,
			LatencyCount:     bucket.latencyCount,
			LatencyTotalMS:   bucket.latencyTotalMS,
			LatencyMinMS:     bucket.latencyMinMS,
			LatencyMaxMS:     bucket.latencyMaxMS,
			LatencyAvgMS:     avg,
			LastError:        bucket.lastError,
			LastObservedUnix: bucket.lastObservedUnix,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Operation == out[j].Operation {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Operation < out[j].Operation
	})
	return out
}
