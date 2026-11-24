package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"go-sni-spoof/pkg/dns"
)

// HandleSOCKS5 processes standard RFC 1928 SOCKS5 client handshakes.
// Returns the resolved remote destination target host:port string.
func HandleSOCKS5(clientConn net.Conn, resolver *dns.Resolver) (string, error) {
	_ = clientConn.SetDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = clientConn.SetDeadline(time.Time{}) }()

	// 1. Method Negotiation — read full 2-byte greeting header (VER + NMETHODS)
	header := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, header); err != nil {
		return "", err
	}
	if header[0] != 0x05 {
		return "", fmt.Errorf("unsupported SOCKS version: 0x%02x", header[0])
	}

	nMethods := int(header[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(clientConn, methods); err != nil {
		return "", err
	}

	// Reply: VER = 5, METHOD = 0 (No Auth)
	if _, err := clientConn.Write([]byte{0x05, 0x00}); err != nil {
		return "", err
	}

	// 2. Request Details
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(clientConn, reqHeader); err != nil {
		return "", err
	}

	if reqHeader[0] != 0x05 {
		return "", errors.New("unsupported SOCKS version")
	}

	cmd := reqHeader[1]
	if cmd != 0x01 { // 0x01 = CONNECT
		// Send command not supported error
		_, _ = clientConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("unsupported SOCKS5 command: 0x%02x", cmd)
	}

	atyp := reqHeader[3]
	var targetHost string

	switch atyp {
	case 0x01: // IPv4
		ipBuf := make([]byte, 4)
		if _, err := io.ReadFull(clientConn, ipBuf); err != nil {
			return "", err
		}
		targetHost = net.IP(ipBuf).String()

	case 0x03: // Domain name
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(clientConn, lenBuf); err != nil {
			return "", err
		}
		domainLen := int(lenBuf[0])
		domainBuf := make([]byte, domainLen)
		if _, err := io.ReadFull(clientConn, domainBuf); err != nil {
			return "", err
		}
		targetHost = string(domainBuf)

	case 0x04: // IPv6
		ipBuf := make([]byte, 16)
		if _, err := io.ReadFull(clientConn, ipBuf); err != nil {
			return "", err
		}
		targetHost = net.IP(ipBuf).String()

	default:
		_, _ = clientConn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("unsupported SOCKS5 address type: 0x%02x", atyp)
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, portBuf); err != nil {
		return "", err
	}
	targetPort := binary.BigEndian.Uint16(portBuf)

	// Resolve hostname with clean DNS if needed
	resolvedHost := targetHost
	if resolver != nil && net.ParseIP(targetHost) == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ip, err := resolver.Resolve(ctx, targetHost)
		cancel()
		if err == nil && ip != nil {
			resolvedHost = ip.String()
		}
	}

	targetAddr := net.JoinHostPort(resolvedHost, strconv.Itoa(int(targetPort)))

	// Send Success Reply
	reply := []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err := clientConn.Write(reply); err != nil {
		return "", err
	}

	return targetAddr, nil
}

