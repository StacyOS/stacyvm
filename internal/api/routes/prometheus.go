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
	for _, provider := range metrics.providerHealth {
		healthy := 0
		if provider.Healthy {
			healthy = 1
		}
		fmt.Fprintf(w, "stacyvm_provider_healthy{provider=%q,default=%q} %d\n", provider.Name, boolLabel(provider.Default), healthy)
	}

	writePrometheusHelp(w, "stacyvm_events_total", "Total events published by the in-process event bus.")
	fmt.Fprintf(w, "stacyvm_events_total %d\n", metrics.eventStats.EventsTotal)
	writePrometheusHelp(w, "stacyvm_event_subscribers", "Current event stream subscriber count.")
	fmt.Fprintf(w, "stacyvm_event_subscribers %d\n", metrics.eventStats.Subscribers)
	writePrometheusHelp(w, "stacyvm_event_history_size", "Current event history item count.")
	fmt.Fprintf(w, "stacyvm_event_history_size %d\n", metrics.eventStats.HistorySize)

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
