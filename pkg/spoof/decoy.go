package spoof

import (
	"crypto/rand"
	"math/big"
	"net"
	"sync"

	"go-sni-spoof/pkg/stats"
	"go-sni-spoof/pkg/tlshelper"
)

// DecoyManager manages decoy SNIs and packet synthesis.
type DecoyManager struct {
	primarySNI string
	sniList    []string
	counter    uint64
	mu         sync.Mutex
	metrics    *stats.Metrics
}

// NewDecoyManager initializes DecoyManager with a list of domains.
func NewDecoyManager(primary string, list []string, m *stats.Metrics) *DecoyManager {
	if primary == "" && len(list) > 0 {
		primary = list[0]
	}
	if primary == "" {
		primary = "speedtest.net"
	}
	if len(list) == 0 {
		list = []string{primary}
	}
	if m == nil {
		m = stats.DefaultMetrics
	}
	return &DecoyManager{
		primarySNI: primary,
		sniList:    list,
		metrics:    m,
	}
}

// GetDecoySNI returns a decoy SNI domain (rotating through list).
func (dm *DecoyManager) GetDecoySNI() string {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if len(dm.sniList) <= 1 {
		return dm.primarySNI
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(dm.sniList))))
	if err == nil {
		return dm.sniList[n.Int64()]
	}

	dm.counter++
	return dm.sniList[dm.counter%uint64(len(dm.sniList))]
}

// GenerateDecoyPayload builds a realistic TLS ClientHello containing the decoy SNI.
func (dm *DecoyManager) GenerateDecoyPayload() []byte {
	sni := dm.GetDecoySNI()
	return tlshelper.GenerateClientHello(sni)
}

// InjectDecoyTTL injects a decoy ClientHello with low TTL so DPI sees the fake SNI
// while the real server drops it.
func (dm *DecoyManager) InjectDecoyTTL(conn net.Conn, lowTTL, delayMs int) error {
	fakePayload := dm.GenerateDecoyPayload()
	err := InjectLowTTLDecoy(conn, fakePayload, lowTTL, delayMs)
	if err == nil {
		dm.metrics.IncInjected()
	}
	return err
}

