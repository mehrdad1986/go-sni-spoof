package proxy

import (
	"context"
	"fmt"
	"net"
	"time"

	"go-sni-spoof/pkg/dns"
	"go-sni-spoof/pkg/spoof"
	"go-sni-spoof/pkg/stats"
	"go-sni-spoof/pkg/tlshelper"
)

// HandleDirectTunnel manages direct TCP port forwarding with SNI spoofing.
func HandleDirectTunnel(clientConn net.Conn, targetAddr string, engine *spoof.Engine, resolver *dns.Resolver, m *stats.Metrics) error {
	// Parse target host and port
	host, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fmt.Errorf("invalid target address %q: %w", targetAddr, err)
	}

	// Resolve target using clean DNS if needed
	resolvedHost := host
	if resolver != nil && net.ParseIP(host) == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ip, rerr := resolver.Resolve(ctx, host)
		cancel()
		if rerr == nil && ip != nil {
			resolvedHost = ip.String()
		}
	}

	dialAddr := net.JoinHostPort(resolvedHost, port)

	// Connect to destination server
	remoteConn, err := net.DialTimeout("tcp", dialAddr, 10*time.Second)
	if err != nil {
		m.IncError()
		return fmt.Errorf("failed to dial target %s: %w", dialAddr, err)
	}

	// Read initial client payload (e.g. TLS ClientHello)
	initialBuf, err := tlshelper.ReadFullClientHello(clientConn)
	if err != nil {
		remoteConn.Close()
		clientConn.Close() // Bug #7 fix: close clientConn to avoid fd leak on error
		m.IncError()
		return fmt.Errorf("failed to read initial handshake: %w", err)
	}

	// Apply DPI evasion to initial payload
	if err := engine.ApplyEvasion(remoteConn, initialBuf); err != nil {
		remoteConn.Close()
		clientConn.Close() // Bug #7 fix: close clientConn to avoid fd leak on error
		m.IncError()
		return fmt.Errorf("failed to apply evasion strategy: %w", err)
	}

	// Forward remaining session
	ForwardSession(clientConn, remoteConn, m)
	return nil
}

