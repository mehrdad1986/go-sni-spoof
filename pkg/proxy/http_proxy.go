package proxy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"go-sni-spoof/pkg/dns"
)

// HandleHTTPConnect processes an HTTP CONNECT proxy request and returns the target address.
func HandleHTTPConnect(clientConn net.Conn, reader *bufio.Reader, resolver *dns.Resolver) (string, error) {
	_ = clientConn.SetDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = clientConn.SetDeadline(time.Time{}) }()

	req, err := http.ReadRequest(reader)
	if err != nil {
		return "", err
	}

	if req.Method != http.MethodConnect {
		return "", fmt.Errorf("expected HTTP CONNECT, got %s", req.Method)
	}

	target := req.RequestURI
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// Default to 443 if port missing
		host = target
		port = "443"
		target = net.JoinHostPort(host, port)
	}

	// Resolve through clean DNS if needed
	resolvedHost := host
	if resolver != nil && net.ParseIP(host) == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ip, rerr := resolver.Resolve(ctx, host)
		cancel()
		if rerr == nil && ip != nil {
			resolvedHost = ip.String()
		}
	}

	targetAddr := net.JoinHostPort(resolvedHost, port)

	// Send HTTP 200 Connection Established
	resp := "HTTP/1.1 200 Connection Established\r\nProxy-Agent: SNI-Spoof/1.0\r\n\r\n"
	if _, err := clientConn.Write([]byte(resp)); err != nil {
		return "", err
	}

	return targetAddr, nil
}

// IsHTTPConnectPrefix checks if initial bytes start with "CONNECT "
func IsHTTPConnectPrefix(buf []byte) bool {
	return strings.HasPrefix(strings.ToUpper(string(buf)), "CONNECT ")
}

