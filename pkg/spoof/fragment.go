package spoof

import (
	"encoding/binary"
	"net"
	"time"

	"go-sni-spoof/pkg/stats"
	"go-sni-spoof/pkg/tlshelper"
)

// FragmentWriter provides methods to fragment TLS handshakes and evade DPI.
type FragmentWriter struct {
	ChunkSize int
	DelayMs   int
	Metrics   *stats.Metrics
}

// NewFragmentWriter initializes a FragmentWriter.
func NewFragmentWriter(chunkSize, delayMs int, m *stats.Metrics) *FragmentWriter {
	if chunkSize <= 0 {
		chunkSize = 3
	}
	if delayMs <= 0 {
		delayMs = 5
	}
	if m == nil {
		m = stats.DefaultMetrics
	}
	return &FragmentWriter{
		ChunkSize: chunkSize,
		DelayMs:   delayMs,
		Metrics:   m,
	}
}

// WriteFragmentedTCP splits a raw TLS ClientHello into two TCP segments
// by forcing a socket flush between chunks.
// FIXED: Increased delay to 50ms for more effective DPI evasion.
func (fw *FragmentWriter) WriteFragmentedTCP(conn net.Conn, data []byte, splitPos int) error {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	if splitPos <= 0 || splitPos >= len(data) {
		splitPos = fw.ChunkSize
	}
	if splitPos >= len(data) {
		splitPos = len(data) / 2
	}

	// 1. Send first chunk
	part1 := data[:splitPos]
	if _, err := conn.Write(part1); err != nil {
		return err
	}

	// 2. Delay to force OS to send separate TCP packet
	// INCREASED from 5ms to 50ms for better DPI evasion
	delayMs := fw.DelayMs
	if delayMs < 50 {
		delayMs = 50
	}
	time.Sleep(time.Duration(delayMs) * time.Millisecond)

	// 3. Send remaining chunk
	part2 := data[splitPos:]
	if _, err := conn.Write(part2); err != nil {
		return err
	}

	fw.Metrics.IncSplit()
	return nil
}

// SplitIntoTLSRecords splits a single TLS ClientHello into two valid TLS records.
// According to RFC 5246 / RFC 8446, a Handshake message may be split across
// consecutive records of the same ContentType. Most DPI systems only parse the first
// record and fail to find the complete SNI.
//
// FIXED: This version properly maintains the handshake message as a continuous stream
// that gets split at the TLS record layer, NOT at the handshake message layer.
func SplitIntoTLSRecords(rawClientHello []byte, splitOffset int) ([][]byte, error) {
	if len(rawClientHello) < 9 || rawClientHello[0] != tlshelper.RecordTypeHandshake {
		return [][]byte{rawClientHello}, tlshelper.ErrNotTLS
	}

	// TLS Record structure:
	// [0]: Content Type (0x16 for Handshake)
	// [1-2]: Protocol Version
	// [3-4]: Record Length (big endian)
	// [5...]: Payload
	
	recordVer := rawClientHello[1:3]
	// Full payload including handshake header
	fullPayload := rawClientHello[5:]

	// Adjust split offset to be within payload bounds
	if splitOffset <= 0 || splitOffset >= len(fullPayload) {
		// Default: split after handshake header (4 bytes) + random (32 bytes) = 36 bytes
		// This ensures handshake type and length are in first record
		splitOffset = 36
		if splitOffset >= len(fullPayload) {
			splitOffset = len(fullPayload) / 2
		}
	}

	// Ensure we don't split within the handshake header itself
	if splitOffset < 4 {
		splitOffset = 4
	}
	if splitOffset > len(fullPayload) {
		splitOffset = len(fullPayload) - 1
	}

	chunk1Payload := fullPayload[:splitOffset]
	chunk2Payload := fullPayload[splitOffset:]

	// Build Record 1 with first chunk
	rec1 := make([]byte, 5+len(chunk1Payload))
	rec1[0] = tlshelper.RecordTypeHandshake
	copy(rec1[1:3], recordVer)
	binary.BigEndian.PutUint16(rec1[3:5], uint16(len(chunk1Payload)))
	copy(rec1[5:], chunk1Payload)

	// Build Record 2 with remaining chunk
	rec2 := make([]byte, 5+len(chunk2Payload))
	rec2[0] = tlshelper.RecordTypeHandshake
	copy(rec2[1:3], recordVer)
	binary.BigEndian.PutUint16(rec2[3:5], uint16(len(chunk2Payload)))
	copy(rec2[5:], chunk2Payload)

	return [][]byte{rec1, rec2}, nil
}

// WriteMultiRecordTLS splits the ClientHello into two TLS records and transmits them
// with a pacing delay to prevent DPI reassembly.
// FIXED: Increased delay to 50ms minimum for better DPI evasion.
func (fw *FragmentWriter) WriteMultiRecordTLS(conn net.Conn, rawClientHello []byte, splitOffset int) error {
	records, err := SplitIntoTLSRecords(rawClientHello, splitOffset)
	if err != nil {
		// Fallback to TCP fragmentation
		return fw.WriteFragmentedTCP(conn, rawClientHello, fw.ChunkSize)
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	// Write Record 1
	if _, err := conn.Write(records[0]); err != nil {
		return err
	}

	// Delay between records - minimum 50ms for effective DPI evasion
	delayMs := fw.DelayMs
	if delayMs < 50 {
		delayMs = 50
	}
	time.Sleep(time.Duration(delayMs) * time.Millisecond)

	// Write Record 2
	if _, err := conn.Write(records[1]); err != nil {
		return err
	}

	fw.Metrics.IncSplit()
	return nil
}

