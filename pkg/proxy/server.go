package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"go-sni-spoof/pkg/config"
	"go-sni-spoof/pkg/dns"
	"go-sni-spoof/pkg/spoof"
	"go-sni-spoof/pkg/stats"
	"go-sni-spoof/pkg/tlshelper"
)

// Server is the multi-protocol DPI-bypassing proxy server.
type Server struct {
	cfg      *config.Config
	engine   *spoof.Engine
	resolver *dns.Resolver
	metrics  *stats.Metrics
	listener net.Listener
	mu       sync.Mutex
	closed   bool
}

// NewServer initializes a new Server.
func NewServer(cfg *config.Config, engine *spoof.Engine, res *dns.Resolver, m *stats.Metrics) *Server {
	if m == nil {
		m = stats.DefaultMetrics
	}
	return &Server{
		cfg:      cfg,
		engine:   engine,
		resolver: res,
		metrics:  m,
	}
}

// Start begins listening and serving connections.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to bind to %s: %w", s.cfg.ListenAddr, err)
	}
	s.listener = ln

	fmt.Printf("[+] Proxy server successfully listening on %s (Mode: %s, Strategy: %s)\n",
		s.cfg.ListenAddr, s.cfg.Mode, s.cfg.Strategy)

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			s.metrics.IncError()
			continue
		}

		go s.handleConnection(conn)
	}
}

// Close gracefully stops the listener.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleConnection(clientConn net.Conn) {
	// If in dedicated tunnel mode with predefined target
	if s.cfg.Mode == "tunnel" && s.cfg.TargetAddr != "" {
		if err := HandleDirectTunnel(clientConn, s.cfg.TargetAddr, s.engine, s.resolver, s.metrics); err != nil {
			if s.cfg.Verbose {
				fmt.Printf("[Tunnel] Error: %v\n", err)
			}
		}
		return
	}

	// Peeking connection protocol using buffered reader
	reader := bufio.NewReader(clientConn)
	peek, err := reader.Peek(1)
	if err != nil {
		clientConn.Close()
		return
	}

	// 1. SOCKS5 Detection (0x05)
	if peek[0] == 0x05 {
		s.serveSOCKS5(clientConn, reader)
		return
	}

	// 2. HTTP CONNECT Detection
	peek8, err := reader.Peek(8)
	if err == nil && IsHTTPConnectPrefix(peek8) {
		s.serveHTTPConnect(clientConn, reader)
		return
	}

	// 3. Direct TLS or predefined target fallback
	if s.cfg.TargetAddr != "" {
		if err := HandleDirectTunnel(clientConn, s.cfg.TargetAddr, s.engine, s.resolver, s.metrics); err != nil {
			if s.cfg.Verbose {
				fmt.Printf("[Direct] Error: %v\n", err)
			}
		}
		return
	}

	// Unknown or unsupported initial protocol without target
	clientConn.Close()
}

func (s *Server) serveSOCKS5(clientConn net.Conn, reader *bufio.Reader) {
	// Read byte 0 (0x05) from buffer
	_, _ = reader.ReadByte()

	// Read NMETHODS
	nMethodsByte, err := reader.ReadByte()
	if err != nil {
		clientConn.Close()
		return
	}
	methods := make([]byte, int(nMethodsByte))
	if _, err := io.ReadFull(reader, methods); err != nil {
		clientConn.Close()
		return
	}

	// Reply NO AUTH (0x05, 0x00)
	if _, err := clientConn.Write([]byte{0x05, 0x00}); err != nil {
		clientConn.Close()
		return
	}

	// SOCKS5 Request
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(reader, reqHeader); err != nil {
		clientConn.Close()
		return
	}

	if reqHeader[0] != 0x05 || reqHeader[1] != 0x01 { // CMD 0x01: CONNECT
		_, _ = clientConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		clientConn.Close()
		return
	}

	atyp := reqHeader[3]
	var targetHost string
	switch atyp {
	case 0x01: // IPv4
		ipBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, ipBuf); err != nil {
			clientConn.Close()
			return
		}
		targetHost = net.IP(ipBuf).String()

	case 0x03: // Domain
		lenByte, err := reader.ReadByte()
		if err != nil {
			clientConn.Close()
			return
		}
		domainBuf := make([]byte, int(lenByte))
		if _, err := io.ReadFull(reader, domainBuf); err != nil {
			clientConn.Close()
			return
		}
		targetHost = string(domainBuf)

	case 0x04: // IPv6
		ipBuf := make([]byte, 16)
		if _, err := io.ReadFull(reader, ipBuf); err != nil {
			clientConn.Close()
			return
		}
		targetHost = net.IP(ipBuf).String()

	default:
		_, _ = clientConn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		clientConn.Close()
		return
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBuf); err != nil {
		clientConn.Close()
		return
	}
	targetPort := int(portBuf[0])<<8 | int(portBuf[1])

	// Resolve destination using clean DNS if host is domain
	resolvedHost := targetHost
	if s.resolver != nil && net.ParseIP(targetHost) == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ip, rerr := s.resolver.Resolve(ctx, targetHost)
		cancel()
		if rerr == nil && ip != nil {
			resolvedHost = ip.String()
		}
	}

	targetAddr := net.JoinHostPort(resolvedHost, fmt.Sprintf("%d", targetPort))

	// Dial target server
	remoteConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		s.metrics.IncError()
		_, _ = clientConn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		clientConn.Close()
		return
	}

	// Send SOCKS5 Success — include bound address bytes (all zeros = 0.0.0.0:0)
	_, _ = clientConn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// Wrap clientConn with reader so unread peeked buffer isn't lost
	wrappedClient := &connWithReader{Conn: clientConn, reader: reader}

	// Read initial payload — may be TLS or plain HTTP
	initialBuf, err := tlshelper.ReadFullClientHello(wrappedClient)
	if err != nil && len(initialBuf) == 0 {
		remoteConn.Close()
		clientConn.Close()
		return
	}

	// Apply DPI evasion to initial payload (handles both TLS and HTTP gracefully)
	if err := s.engine.ApplyEvasion(remoteConn, initialBuf); err != nil {
		remoteConn.Close()
		clientConn.Close()
		return
	}

	ForwardSession(wrappedClient, remoteConn, s.metrics)
}

func (s *Server) serveHTTPConnect(clientConn net.Conn, reader *bufio.Reader) {
	targetAddr, err := HandleHTTPConnect(clientConn, reader, s.resolver)
	if err != nil {
		clientConn.Close()
		return
	}

	// Dial target server
	remoteConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		s.metrics.IncError()
		clientConn.Close()
		return
	}

	// Wrap client connection with remaining buffer
	wrappedClient := &connWithReader{Conn: clientConn, reader: reader}

	// Read initial payload — may be TLS ClientHello or plain HTTP
	initialBuf, err := tlshelper.ReadFullClientHello(wrappedClient)
	if err != nil && len(initialBuf) == 0 {
		remoteConn.Close()
		clientConn.Close()
		return
	}

	// Apply DPI evasion to initial payload
	if err := s.engine.ApplyEvasion(remoteConn, initialBuf); err != nil {
		remoteConn.Close()
		clientConn.Close()
		return
	}

	ForwardSession(wrappedClient, remoteConn, s.metrics)
}

// connWithReader combines net.Conn and bufio.Reader so that any buffered
// (already-peeked) bytes are drained before reading from the raw socket.
type connWithReader struct {
	net.Conn
	reader *bufio.Reader
}

func (c *connWithReader) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

// CloseWrite propagates TCP half-close through to the underlying TCPConn.
func (c *connWithReader) CloseWrite() error {
	if tcp, ok := c.Conn.(*net.TCPConn); ok {
		return tcp.CloseWrite()
	}
	return nil
}

