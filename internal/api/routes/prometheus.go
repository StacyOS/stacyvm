package routes

import (
	"fmt"
	"io"
	"sort"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
)

func writePrometheusMetrics(w io.Writer, metrics systemMetricsSnapshot) {
	writePrometheusHelp(w, "stacyvm_uptime_seconds", "StacyVM API process uptime in seconds.")
	fmt.Fprintf(w, "stacyvm_uptime_seconds %d\n", int64(metrics.uptime.Seconds()))

	writePrometheusHelp(w, "stacyvm_runtime_goroutines", "Current number of goroutines.")
	fmt.Fprintf(w, "stacyvm_runtime_goroutines %d\n", metrics.goroutines)

	writePrometheusHelp(w, "stacyvm_runtime_memory_alloc_bytes", "Current allocated memory in bytes.")
	fmt.Fprintf(w, "stacyvm_runtime_memory_alloc_bytes %d\n", metrics.memoryAlloc)

	writePrometheusHelp(w, "stacyvm_runtime_memory_sys_bytes", "Total memory obtained from the OS in bytes.")
	fmt.Fprintf(w, "stacyvm_runtime_memory_sys_bytes %d\n", metrics.memorySys)

	writePrometheusHelp(w, "stacyvm_runtime_gc_cycles_total", "Total completed GC cycles.")
	fmt.Fprintf(w, "stacyvm_runtime_gc_cycles_total %d\n", metrics.gcCycles)

	writePrometheusHelp(w, "stacyvm_sandboxes_total", "Current sandbox records by state and provider.")
	states := sortedKeys(metrics.sandboxesByState)
	for _, state := range states {
		fmt.Fprintf(w, "stacyvm_sandboxes_total{state=%q} %d\n", state, metrics.sandboxesByState[state])
	}
	providers := sortedKeys(metrics.sandboxesByProvider)
	for _, provider := range providers {
		fmt.Fprintf(w, "stacyvm_sandboxes_by_provider_total{provider=%q} %d\n", provider, metrics.sandboxesByProvider[provider])
	}

	writePrometheusHelp(w, "stacyvm_provider_healthy", "Provider health status where 1 is healthy and 0 is unhealthy.")
	writePrometheusHelp(w, "stacyvm_provider_health_latency_milliseconds", "Provider health check latency in milliseconds.")
	writePrometheusHelp(w, "stacyvm_provider_runtime_sandboxes", "Runtime sandboxes discovered directly from providers.")
	for _, provider := range metrics.providerHealth {
		healthy := 0
		if provider.Healthy {
			healthy = 1
		}
		fmt.Fprintf(w, "stacyvm_provider_healthy{provider=%q,default=%q} %d\n", provider.Name, boolLabel(provider.Default), healthy)
		fmt.Fprintf(w, "stacyvm_provider_health_latency_milliseconds{provider=%q} %d\n", provider.Name, provider.LatencyMS)
		if provider.RuntimeCount != nil {
			fmt.Fprintf(w, "stacyvm_provider_runtime_sandboxes{provider=%q} %d\n", provider.Name, *provider.RuntimeCount)
		}
	}

	if metrics.workerSummary != nil {
		writePrometheusHelp(w, "stacyvm_workers_total", "Registered worker count by status bucket.")
		for _, key := range []string{"total", "online", "stale", "unhealthy"} {
			fmt.Fprintf(w, "stacyvm_workers_total{status=%q} %d\n", key, intMetric(metrics.workerSummary[key]))
		}
	}

	writePrometheusHelp(w, "stacyvm_events_total", "Total events published by the in-process event bus.")
	fmt.Fprintf(w, "stacyvm_events_total %d\n", metrics.eventStats.EventsTotal)
	writePrometheusHelp(w, "stacyvm_event_subscribers", "Current event stream subscriber count.")
	fmt.Fprintf(w, "stacyvm_event_subscribers %d\n", metrics.eventStats.Subscribers)
	writePrometheusHelp(w, "stacyvm_event_history_size", "Current event history item count.")
	fmt.Fprintf(w, "stacyvm_event_history_size %d\n", metrics.eventStats.HistorySize)

	writePrometheusHelp(w, "stacyvm_spawn_queue_depth", "Current number of spawn requests waiting for capacity.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_depth %d\n", metrics.schedulerStatus.SpawnQueueDepth)
	writePrometheusHelp(w, "stacyvm_spawn_queue_capacity", "Configured maximum number of queued spawn requests.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_capacity %d\n", metrics.schedulerStatus.MaxSpawnQueue)
	writePrometheusHelp(w, "stacyvm_spawn_queue_enqueued_total", "Total spawn requests admitted into the capacity wait queue.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_enqueued_total %d\n", metrics.schedulerStatus.SpawnQueuedTotal)
	writePrometheusHelp(w, "stacyvm_spawn_queue_dequeued_total", "Total spawn requests released from the capacity wait queue.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_dequeued_total %d\n", metrics.schedulerStatus.SpawnDequeuedTotal)
	writePrometheusHelp(w, "stacyvm_spawn_queue_timeout_total", "Total spawn requests that timed out while waiting in the capacity queue.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_timeout_total %d\n", metrics.schedulerStatus.SpawnQueueTimeouts)
	writePrometheusHelp(w, "stacyvm_spawn_queue_wait_milliseconds_sum", "Total observed spawn queue wait time in milliseconds.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_wait_milliseconds_sum %d\n", metrics.schedulerStatus.SpawnQueueWaitTotalMS)
	writePrometheusHelp(w, "stacyvm_spawn_queue_wait_milliseconds_count", "Total observed spawn queue wait samples.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_wait_milliseconds_count %d\n", metrics.schedulerStatus.SpawnQueueWaitCount)
	writePrometheusHelp(w, "stacyvm_spawn_queue_wait_milliseconds_max", "Maximum observed spawn queue wait time in milliseconds.")
	fmt.Fprintf(w, "stacyvm_spawn_queue_wait_milliseconds_max %d\n", metrics.schedulerStatus.SpawnQueueWaitMaxMS)

	writePrometheusHelp(w, "stacyvm_owner_quotas_total", "Total configured owner quota policies.")
	fmt.Fprintf(w, "stacyvm_owner_quotas_total %d\n", metrics.quotaSummary.Total)
	writePrometheusHelp(w, "stacyvm_owner_quota_overrides_total", "Total configured owner quota overrides by override type.")
	fmt.Fprintf(w, "stacyvm_owner_quota_overrides_total{type=%q} %d\n", "max_sandboxes", metrics.quotaSummary.WithMaxSandboxes)
	fmt.Fprintf(w, "stacyvm_owner_quota_overrides_total{type=%q} %d\n", "max_ttl", metrics.quotaSummary.WithMaxTTL)
	fmt.Fprintf(w, "stacyvm_owner_quota_overrides_total{type=%q} %d\n", "max_exec_timeout", metrics.quotaSummary.WithMaxExecTimeout)

	writePrometheusHelp(w, "stacyvm_rate_limit_allowed_total", "Total API requests allowed by the in-process rate limiter.")
	fmt.Fprintf(w, "stacyvm_rate_limit_allowed_total %d\n", metrics.rateLimitStats.AllowedTotal)
	writePrometheusHelp(w, "stacyvm_rate_limit_blocked_total", "Total API requests blocked by the in-process rate limiter.")
	fmt.Fprintf(w, "stacyvm_rate_limit_blocked_total %d\n", metrics.rateLimitStats.LimitedTotal)
	writePrometheusHelp(w, "stacyvm_rate_limit_evicted_buckets_total", "Total inactive rate-limit buckets evicted from memory.")
	fmt.Fprintf(w, "stacyvm_rate_limit_evicted_buckets_total %d\n", metrics.rateLimitStats.EvictedTotal)
	writePrometheusHelp(w, "stacyvm_rate_limit_active_buckets", "Current number of active rate-limit buckets.")
	fmt.Fprintf(w, "stacyvm_rate_limit_active_buckets %d\n", metrics.rateLimitStats.ActiveBuckets)

	writeOperationMetrics(w, metrics.operationMetrics)
}

func writeOperationMetrics(w io.Writer, operationMetrics []orchestrator.OperationMetrics) {
	writePrometheusHelp(w, "stacyvm_operation_success_total", "Total successful operations by operation and provider.")
	writePrometheusHelp(w, "stacyvm_operation_failure_total", "Total failed operations by operation and provider.")
	writePrometheusHelp(w, "stacyvm_operation_latency_milliseconds_sum", "Total operation latency in milliseconds by operation and provider.")
	writePrometheusHelp(w, "stacyvm_operation_latency_milliseconds_count", "Total observed operation latency samples by operation and provider.")
	writePrometheusHelp(w, "stacyvm_operation_latency_milliseconds_max", "Maximum observed operation latency in milliseconds by operation and provider.")
	for _, metric := range operationMetrics {
		labels := fmt.Sprintf("operation=%q,provider=%q", metric.Operation, metric.Provider)
		fmt.Fprintf(w, "stacyvm_operation_success_total{%s} %d\n", labels, metric.SuccessTotal)
		fmt.Fprintf(w, "stacyvm_operation_failure_total{%s} %d\n", labels, metric.FailureTotal)
		fmt.Fprintf(w, "stacyvm_operation_latency_milliseconds_sum{%s} %d\n", labels, metric.LatencyTotalMS)
		fmt.Fprintf(w, "stacyvm_operation_latency_milliseconds_count{%s} %d\n", labels, metric.LatencyCount)
		fmt.Fprintf(w, "stacyvm_operation_latency_milliseconds_max{%s} %d\n", labels, metric.LatencyMaxMS)
	}
}

func writePrometheusHelp(w io.Writer, name, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", name)
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func intMetric(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
