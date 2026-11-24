package spoof

import (
	"bytes"
	"net"
	"strings"
	"time"
)

// IsHTTPRequest checks if data looks like an unencrypted HTTP/1.x request.
func IsHTTPRequest(data []byte) bool {
	methods := [][]byte{
		[]byte("GET "),
		[]byte("POST "),
		[]byte("HEAD "),
		[]byte("PUT "),
		[]byte("DELETE "),
		[]byte("CONNECT "),
		[]byte("OPTIONS "),
	}
	for _, m := range methods {
		if bytes.HasPrefix(data, m) {
			return true
		}
	}
	return false
}

// ManipulateHTTPHost modifies HTTP headers to confuse DPI filters:
// - Alters "Host:" to mixed-case "hOsT:" or "Host: " with extra whitespace
// - Fragments the request across packets right at the Host header
func ManipulateHTTPHost(conn net.Conn, reqData []byte, delayMs int) error {
	if delayMs <= 0 {
		delayMs = 5
	}

	// Find "Host:" header
	idx := bytes.Index(bytes.ToLower(reqData), []byte("host:"))
	if idx == -1 {
		_, err := conn.Write(reqData)
		return err
	}

	// 1. Send request prefix up to middle of Host header
	splitPos := idx + 2 // Split right in "Ho" | "st:"
	part1 := reqData[:splitPos]
	part2 := reqData[splitPos:]

	if _, err := conn.Write(part1); err != nil {
		return err
	}

	time.Sleep(time.Duration(delayMs) * time.Millisecond)

	_, err := conn.Write(part2)
	return err
}

// ObfuscateHostHeader replaces "Host: example.com" with "hOsT: example.com"
func ObfuscateHostHeader(reqData []byte) []byte {
	str := string(reqData)
	lines := strings.Split(str, "\r\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			val := strings.TrimSpace(line[5:])
			// Replace with mixed-case Host
			lines[i] = "hOsT: " + val
			break
		}
	}
	return []byte(strings.Join(lines, "\r\n"))
}

