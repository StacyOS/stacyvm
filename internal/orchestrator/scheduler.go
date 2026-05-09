package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/store"
)

const workerHeartbeatStaleAfter = 2 * time.Minute

type workerPlacement struct {
	SelectedID string
	Eligible   int
	Reason     string
}

type workerCapacity struct {
	MaxSandboxes int `json:"max_sandboxes"`
}

func (m *Manager) evaluateWorkerPlacement(ctx context.Context, provider string, sandboxes []*store.SandboxRecord) workerPlacement {
	workers, err := m.store.ListWorkers(ctx)
	if err != nil || len(workers) == 0 {
		return workerPlacement{SelectedID: m.workerID, Eligible: 1, Reason: "local_fallback"}
	}

	activeByWorker := make(map[string]int)
	for _, sb := range sandboxes {
		if SandboxState(sb.State) == StateDestroyed {
			continue
		}
		workerID := strings.TrimSpace(sb.WorkerID)
		if workerID == "" {
			workerID = m.workerID
		}
		activeByWorker[workerID]++
	}

	now := time.Now().UTC()
	bestID := ""
	bestCount := 0
	eligible := 0
	for _, worker := range workers {
		workerID := strings.TrimSpace(worker.ID)
		if workerID == "" || !strings.EqualFold(worker.Status, "online") {
			continue
		}
		if now.Sub(worker.LastHeartbeat) > workerHeartbeatStaleAfter {
			continue
		}
		if provider != "" && !workerSupportsProvider(worker, provider) {
			continue
		}
		count := activeByWorker[workerID]
		if cap := workerMaxSandboxes(worker); cap > 0 && count >= cap {
			continue
		}
		eligible++
		if workerID == m.workerID {
			bestID = workerID
			bestCount = count
			continue
		}
		if bestID == "" || count < bestCount {
			bestID = workerID
			bestCount = count
		}
	}

	if bestID == "" {
		return workerPlacement{Reason: "no_eligible_worker"}
	}
	return workerPlacement{SelectedID: bestID, Eligible: eligible}
}

func workerSupportsProvider(worker *store.WorkerRecord, provider string) bool {
	var providers []string
	if err := json.Unmarshal([]byte(worker.Providers), &providers); err != nil {
		return false
	}
	if len(providers) == 0 {
		return true
	}
	for _, name := range providers {
		if strings.EqualFold(strings.TrimSpace(name), provider) {
			return true
		}
	}
	return false
}

func workerMaxSandboxes(worker *store.WorkerRecord) int {
	var capacity workerCapacity
	if err := json.Unmarshal([]byte(worker.Capacity), &capacity); err != nil {
		return 0
	}
	return capacity.MaxSandboxes
}
