package spoof

import (
	"net"
	"time"
)

// SetSocketTTL sets the IP Time-To-Live (hop count) on an active TCP connection.
// This is used for decoy packet injection where the decoy packet reaches the ISP DPI
// middlebox (e.g. 3-6 hops away) but expires before reaching the destination server.
func SetSocketTTL(conn net.Conn, ttl int) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}

	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return err
	}

	var sockErr error
	err = rawConn.Control(func(fd uintptr) {
		sockErr = setSockOptTTL(fd, ttl)
	})
	if err != nil {
		return err
	}
	return sockErr
}

// InjectLowTTLDecoy sends a fake TLS ClientHello packet with a low TTL so it is
// processed by the ISP's DPI inspection engine but dropped before reaching the target.
func InjectLowTTLDecoy(conn net.Conn, fakePayload []byte, lowTTL int, delayMs int) error {
	if lowTTL <= 0 {
		lowTTL = 5
	}
	if delayMs <= 0 {
		delayMs = 5
	}

	// 1. Set socket TTL to low value (reaches ISP middlebox, dies before destination)
	if err := SetSocketTTL(conn, lowTTL); err != nil {
		return err
	}

	// 2. Transmit the fake ClientHello with spoofed SNI
	if _, err := conn.Write(fakePayload); err != nil {
		return err
	}

	// Micro-sleep to allow packet to be sent on wire
	time.Sleep(time.Duration(delayMs) * time.Millisecond)

	// 3. Restore standard TTL (64) for legitimate traffic
	return SetSocketTTL(conn, 64)
}
