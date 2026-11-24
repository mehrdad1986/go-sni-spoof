package spoof

import (
	"fmt"
	"net"
	"runtime"
	"time"

	"go-sni-spoof/pkg/config"
	"go-sni-spoof/pkg/stats"
	"go-sni-spoof/pkg/tlshelper"
)

// Engine orchestrates DPI bypass strategies on outbound connections.
type Engine struct {
	cfg            *config.Config
	metrics        *stats.Metrics
	fragmenter     *FragmentWriter
	decoyMgr       *DecoyManager
	winDivert      *WinDivertInjector
	useWinDivert   bool
	connectIP      net.IP
	connectPort    uint16
}

// NewEngine creates an initialized Engine.
func NewEngine(cfg *config.Config, m *stats.Metrics) *Engine {
	if m == nil {
		m = stats.DefaultMetrics
	}
	
	engine := &Engine{
		cfg:        cfg,
		metrics:    m,
		fragmenter: NewFragmentWriter(cfg.FragmentSize, cfg.FragmentDelayMs, m),
		decoyMgr:   NewDecoyManager(cfg.FakeSNI, cfg.FakeSNIList, m),
	}
	
	// Try to initialize WinDivert on Windows for advanced injection
	if runtime.GOOS == "windows" && cfg.TargetAddr != "" {
		host, port, err := net.SplitHostPort(cfg.TargetAddr)
		if err == nil {
			ip := net.ParseIP(host)
			if ip != nil && ip.To4() != nil {
				engine.connectIP = ip.To4()
				engine.connectPort = uint16(parsePort(port))
				
				injector, err := NewWinDivertInjector(engine.connectIP, engine.connectPort, m, cfg.Verbose)
				if err == nil {
					engine.winDivert = injector
					engine.useWinDivert = true
					if cfg.Verbose {
						fmt.Printf("[+] WinDivert initialized for advanced packet injection\n")
					}
					// Start WinDivert packet capture in background
					go injector.Start()
				} else if cfg.Verbose {
					fmt.Printf("[!] WinDivert init failed: %v (falling back to userspace)\n", err)
				}
			}
		}
	}
	
	return engine
}

func parsePort(portStr string) int {
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// ApplyEvasion handles the initial handshake payload for an outbound connection.
// It detects TLS, parses SNI, and executes the configured evasion strategy.
// This is the FIXED version that properly handles WinDivert mode with real ClientHello buffering.
func (e *Engine) ApplyEvasion(remoteConn net.Conn, initialPayload []byte) error {
	// 1. Check for TLS Handshake
	if tlshelper.IsTLSHandshake(initialPayload) {
		info, err := tlshelper.ParseClientHello(initialPayload)
		if err == nil && info.SNI != "" {
			e.metrics.IncSNI()
			if e.cfg.Verbose {
				fmt.Printf("[TLS] Detected SNI: %s (RecordLen: %d, Offset: %d)\n", info.SNI, info.RecordLength, info.SNIOffset)
			}
		}

		// Use WinDivert wrong-sequence injection if available
		if e.useWinDivert && e.winDivert != nil {
			return e.executeWinDivertInjection(remoteConn, initialPayload, info)
		}

		// Otherwise use userspace strategies
		strategy := e.cfg.Strategy
		switch strategy {
		case "fragment":
			return e.executeFragmentation(remoteConn, initialPayload, info)

		case "decoy":
			return e.executeDecoyInjection(remoteConn, initialPayload, info)

		case "ttl":
			return e.executeTTLDecoy(remoteConn, initialPayload, info)

		case "combo":
			fallthrough
		default:
			return e.executeCombo(remoteConn, initialPayload, info)
		}
	}

	// 2. Check for Plain HTTP
	if IsHTTPRequest(initialPayload) {
		if e.cfg.Verbose {
			fmt.Printf("[HTTP] Plaintext HTTP request detected, applying host obfuscation\n")
		}
		return ManipulateHTTPHost(remoteConn, initialPayload, e.cfg.FragmentDelayMs)
	}

	// 3. Fallback for other data: standard write
	_, err := remoteConn.Write(initialPayload)
	return err
}

// executeWinDivertInjection performs wrong-sequence decoy injection using WinDivert.
// This is the CRITICAL FIX: it properly buffers the real ClientHello and waits for decoy confirmation.
func (e *Engine) executeWinDivertInjection(remoteConn net.Conn, realClientHello []byte, info *tlshelper.ClientHelloInfo) error {
	tcpConn, ok := remoteConn.(*net.TCPConn)
	if !ok {
		// Fallback to userspace if not a TCP connection
		return e.executeCombo(remoteConn, realClientHello, info)
	}

	// Get local address
	localAddr := tcpConn.LocalAddr().(*net.TCPAddr)
	remoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	
	// Create connection tracking entry
	connID := ConnID{
		SrcPort: uint16(localAddr.Port),
		DstPort: uint16(remoteAddr.Port),
	}
	copy(connID.SrcIP[:], localAddr.IP.To4())
	copy(connID.DstIP[:], remoteAddr.IP.To4())

	fakeConn := &FakeConnection{
		Monitor:     true,
		SynSeq:      -1,
		SynAckSeq:   -1,
		ID:          connID,
		FakeData:    GenerateFakeClientHello(e.cfg.FakeSNI),
		SchFakeSent: false,
		FakeSent:    false,
		T2aChan:     make(chan string, 1),
		RealCHReady: make(chan []byte, 1),
	}

	// Register connection with WinDivert
	if !e.winDivert.register(fakeConn) {
		// Port conflict, fallback to userspace
		if e.cfg.Verbose {
			fmt.Printf("[!] Port registration failed, using userspace fallback\n")
		}
		return e.executeCombo(remoteConn, realClientHello, info)
	}
	defer e.winDivert.unregister(fakeConn)

	// Send real ClientHello to injection goroutine
	fakeConn.RealCHReady <- realClientHello

	// Wait for decoy injection confirmation
	select {
	case status := <-fakeConn.T2aChan:
		if status == "fake_data_ack_recv" {
			if e.cfg.Verbose {
				fmt.Printf("[+] Fake ClientHello ACK confirmed - DPI bypass successful!\n")
			}
			// Now send the real ClientHello
			_, err := remoteConn.Write(realClientHello)
			return err
		} else {
			if e.cfg.Verbose {
				fmt.Printf("[!] WinDivert status: %s, falling back to userspace\n", status)
			}
			// Fallback to userspace fragmentation
			return e.executeCombo(remoteConn, realClientHello, info)
		}
	case <-time.After(time.Duration(e.cfg.HandshakeTimeoutMs) * time.Millisecond):
		if e.cfg.Verbose {
			fmt.Printf("[!] WinDivert injection timeout, falling back to userspace\n")
		}
		// Timeout, fallback to userspace
		return e.executeCombo(remoteConn, realClientHello, info)
	}
}

// Cleanup releases WinDivert resources
func (e *Engine) Cleanup() {
	if e.winDivert != nil {
		e.winDivert.Close()
	}
}

func (e *Engine) executeFragmentation(conn net.Conn, data []byte, info *tlshelper.ClientHelloInfo) error {
	splitPos := e.cfg.FragmentSize
	if info != nil && info.SNIOffset > 0 {
		// Split right inside the SNI domain
		splitPos = info.SNIOffset + 1
	}

	// Try TLS Multi-Record split first (extremely reliable on fast networks)
	err := e.fragmenter.WriteMultiRecordTLS(conn, data, splitPos)
	if err != nil {
		// Fallback to TCP packet segmentation
		return e.fragmenter.WriteFragmentedTCP(conn, data, e.cfg.FragmentSize)
	}
	return nil
}

func (e *Engine) executeDecoyInjection(conn net.Conn, data []byte, info *tlshelper.ClientHelloInfo) error {
	// Inject low-TTL decoy packet with fake SNI
	_ = e.decoyMgr.InjectDecoyTTL(conn, e.cfg.FakeTTL, e.cfg.FragmentDelayMs)
	// Send real payload with fragmentation
	return e.executeFragmentation(conn, data, info)
}

func (e *Engine) executeTTLDecoy(conn net.Conn, data []byte, info *tlshelper.ClientHelloInfo) error {
	// Send fake packet with restricted hop limit
	err := e.decoyMgr.InjectDecoyTTL(conn, e.cfg.FakeTTL, e.cfg.FragmentDelayMs)
	if err != nil && e.cfg.Verbose {
		fmt.Printf("[TTL] Warning: low-TTL injection failed: %v\n", err)
	}
	// Transmit real ClientHello
	_, err = conn.Write(data)
	return err
}

func (e *Engine) executeCombo(conn net.Conn, data []byte, info *tlshelper.ClientHelloInfo) error {
	// 1. Send decoy packet with low TTL to poison DPI flow tracker
	_ = e.decoyMgr.InjectDecoyTTL(conn, e.cfg.FakeTTL, e.cfg.FragmentDelayMs)

	// 2. Transmit real ClientHello using TLS record-layer fragmentation
	splitPos := e.cfg.FragmentSize
	if info != nil && info.SNIOffset > 0 {
		splitPos = info.SNIOffset + 2
	}
	return e.fragmenter.WriteMultiRecordTLS(conn, data, splitPos)
}

