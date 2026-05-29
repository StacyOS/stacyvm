package routes

// hoststats.go — real host telemetry for the TUI's ribbon + HOST TELEMETRY
// module. A background sampler (gopsutil) refreshes ~1s; GET /system/stats
// returns the latest snapshot without blocking the request.

import (
	"net/http"
	"sync"
	"time"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

type hostStatsSnapshot struct {
	CPUPct   float64 `json:"cpu_pct"`
	MemPct   float64 `json:"mem_pct"`
	DiskPct  float64 `json:"disk_pct"`
	NetRxBps float64 `json:"net_rx_bps"`
	NetTxBps float64 `json:"net_tx_bps"`
	Load1    float64 `json:"load1"`
}

var (
	hostStatsMu   sync.RWMutex
	hostStatsSnap hostStatsSnapshot
	hostStatsOnce sync.Once
)

// startHostSampler launches the sampling goroutine exactly once (lazily, on the
// first /system/stats request) so test binaries that never hit it pay nothing.
func startHostSampler() {
	hostStatsOnce.Do(func() {
		go func() {
			var prevRx, prevTx uint64
			var prevTime time.Time
			for {
				// cpu.Percent blocks for the interval and returns the average
				// busy% over that window — this is also our ~1s sample cadence.
				snap := hostStatsSnapshot{}
				if pcts, err := cpu.Percent(time.Second, false); err == nil && len(pcts) > 0 {
					snap.CPUPct = pcts[0]
				}
				if vm, err := mem.VirtualMemory(); err == nil {
					snap.MemPct = vm.UsedPercent
				}
				if du, err := disk.Usage("/"); err == nil {
					snap.DiskPct = du.UsedPercent
				}
				if la, err := load.Avg(); err == nil {
					snap.Load1 = la.Load1
				}
				now := time.Now()
				if cs, err := gnet.IOCounters(false); err == nil && len(cs) > 0 {
					rx, tx := cs[0].BytesRecv, cs[0].BytesSent
					if !prevTime.IsZero() {
						if dt := now.Sub(prevTime).Seconds(); dt > 0 {
							if rx >= prevRx {
								snap.NetRxBps = float64(rx-prevRx) / dt
							}
							if tx >= prevTx {
								snap.NetTxBps = float64(tx-prevTx) / dt
							}
						}
					}
					prevRx, prevTx, prevTime = rx, tx, now
				}

				hostStatsMu.Lock()
				hostStatsSnap = snap
				hostStatsMu.Unlock()
			}
		}()
	})
}

func currentHostStats() hostStatsSnapshot {
	hostStatsMu.RLock()
	defer hostStatsMu.RUnlock()
	return hostStatsSnap
}

// HostStats handles GET /api/v1/system/stats.
//
//	@Summary		Host telemetry
//	@Description	Return live host CPU/MEM/DISK/NET/load for the dashboard
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	hostStatsSnapshot
//	@Security		ApiKeyAuth
//	@Router			/system/stats [get]
func (s *SystemRoutes) HostStats(w http.ResponseWriter, r *http.Request) {
	startHostSampler()
	httputil.WriteJSON(w, http.StatusOK, currentHostStats())
}
