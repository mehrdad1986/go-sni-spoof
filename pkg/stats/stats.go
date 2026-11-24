package stats

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Metrics holds the atomic performance counters for the proxy.
// IMPORTANT: int64 fields must appear first (or be 8-byte aligned) for
// correct atomic operation on 32-bit architectures.
type Metrics struct {
	ActiveConnections int64  // must stay first for 32-bit atomic alignment
	TotalConnections  uint64
	BytesUploaded     uint64
	BytesDownloaded   uint64
	PacketsInjected   uint64
	HandshakesSplit   uint64
	SNIsDetected      uint64
	Errors            uint64
	StartTime         time.Time
}

// Global default metrics instance
var DefaultMetrics = NewMetrics()

// NewMetrics initializes a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime: time.Now(),
	}
}

// IncConnections increments total and active connections.
func (m *Metrics) IncConnections() {
	atomic.AddUint64(&m.TotalConnections, 1)
	atomic.AddInt64(&m.ActiveConnections, 1)
}

// DecConnections decrements active connections.
func (m *Metrics) DecConnections() {
	atomic.AddInt64(&m.ActiveConnections, -1)
}

// AddUpload adds bytes to the upload counter.
func (m *Metrics) AddUpload(n int) {
	if n > 0 {
		atomic.AddUint64(&m.BytesUploaded, uint64(n))
	}
}

// AddDownload adds bytes to the download counter.
func (m *Metrics) AddDownload(n int) {
	if n > 0 {
		atomic.AddUint64(&m.BytesDownloaded, uint64(n))
	}
}

// IncInjected increments fake packet injection counter.
func (m *Metrics) IncInjected() {
	atomic.AddUint64(&m.PacketsInjected, 1)
}

// IncSplit increments handshake fragmentation counter.
func (m *Metrics) IncSplit() {
	atomic.AddUint64(&m.HandshakesSplit, 1)
}

// IncSNI increments detected SNI counter.
func (m *Metrics) IncSNI() {
	atomic.AddUint64(&m.SNIsDetected, 1)
}

// IncError increments error counter.
func (m *Metrics) IncError() {
	atomic.AddUint64(&m.Errors, 1)
}

// Snapshot returns a formatted status string.
func (m *Metrics) Snapshot() string {
	uptime := time.Since(m.StartTime).Truncate(time.Second)
	return fmt.Sprintf(
		"Uptime: %s | Active: %d | Total: %d | Up: %s | Down: %s | SNIs: %d | Injected: %d | Split: %d | Err: %d",
		uptime,
		atomic.LoadInt64(&m.ActiveConnections),
		atomic.LoadUint64(&m.TotalConnections),
		FormatBytes(atomic.LoadUint64(&m.BytesUploaded)),
		FormatBytes(atomic.LoadUint64(&m.BytesDownloaded)),
		atomic.LoadUint64(&m.SNIsDetected),
		atomic.LoadUint64(&m.PacketsInjected),
		atomic.LoadUint64(&m.HandshakesSplit),
		atomic.LoadUint64(&m.Errors),
	)
}

// FormatBytes formats byte counts into human-readable strings.
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

