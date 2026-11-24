package tlshelper

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

var (
	ErrNotTLS           = errors.New("not a valid TLS record")
	ErrNotClientHello   = errors.New("not a TLS ClientHello message")
	ErrIncompleteRecord = errors.New("incomplete TLS record")
	ErrSNINotFound      = errors.New("SNI extension not found in ClientHello")
)

// IsTLSHandshake checks if the given buffer begins with a TLS Handshake record header.
func IsTLSHandshake(buf []byte) bool {
	if len(buf) < 3 {
		return false
	}
	// Record type 22 (Handshake) and version 3.x (SSL 3.0 / TLS 1.0 - 1.3)
	return buf[0] == RecordTypeHandshake && buf[1] == 3 && buf[2] <= 3
}

// ReadFullClientHello reads enough bytes from a connection to assemble a complete TLS ClientHello.
// It handles fragmentation across multiple TCP packets seamlessly.
func ReadFullClientHello(conn net.Conn) ([]byte, error) {
	// First read 5 bytes of TLS record header
	header := make([]byte, 5)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, header); err != nil {
		_ = conn.SetReadDeadline(time.Time{})
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Time{})

	if !IsTLSHandshake(header) {
		return header, ErrNotTLS
	}

	payloadLen := int(binary.BigEndian.Uint16(header[3:5]))
	if payloadLen <= 0 || payloadLen > 16384+2048 {
		return header, ErrNotTLS
	}

	fullBuf := make([]byte, 5+payloadLen)
	copy(fullBuf[:5], header)

	// Read remaining record payload
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, fullBuf[5:]); err != nil {
		_ = conn.SetReadDeadline(time.Time{})
		return fullBuf, err
	}
	_ = conn.SetReadDeadline(time.Time{})

	return fullBuf, nil
}

// ParseClientHello parses a raw TLS ClientHello packet and extracts SNI metadata.
func ParseClientHello(data []byte) (*ClientHelloInfo, error) {
	if len(data) < 5 {
		return nil, ErrIncompleteRecord
	}

	if data[0] != RecordTypeHandshake {
		return nil, ErrNotTLS
	}

	recordLen := int(binary.BigEndian.Uint16(data[3:5]))
	if len(data) < 5+recordLen {
		return nil, ErrIncompleteRecord
	}

	// Handshake payload starts at byte 5
	hs := data[5 : 5+recordLen]
	if len(hs) < 4 {
		return nil, ErrIncompleteRecord
	}

	if hs[0] != HandshakeTypeClientHello {
		return nil, ErrNotClientHello
	}

	hsLen := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
	if len(hs) < 4+hsLen {
		return nil, ErrIncompleteRecord
	}

	info := &ClientHelloInfo{
		RawPayload:      data[:5+recordLen],
		RecordLength:    5 + recordLen,
		HandshakeLength: hsLen,
		IsComplete:      true,
	}

	// Skip Handshake Header (4 bytes) + Client Version (2 bytes) + Random (32 bytes)
	offset := 4 + 2 + 32
	if len(hs) < offset+1 {
		return info, ErrIncompleteRecord
	}

	// Session ID
	sessionIDLen := int(hs[offset])
	offset += 1 + sessionIDLen
	if len(hs) < offset+2 {
		return info, ErrIncompleteRecord
	}

	// Cipher Suites
	cipherSuitesLen := int(binary.BigEndian.Uint16(hs[offset : offset+2]))
	offset += 2 + cipherSuitesLen
	if len(hs) < offset+1 {
		return info, ErrIncompleteRecord
	}

	// Compression Methods
	compressionMethodsLen := int(hs[offset])
	offset += 1 + compressionMethodsLen
	if len(hs) < offset+2 {
		// No extensions
		return info, ErrSNINotFound
	}

	// Extensions
	extensionsLen := int(binary.BigEndian.Uint16(hs[offset : offset+2]))
	offset += 2
	extensionsEnd := offset + extensionsLen
	if len(hs) < extensionsEnd {
		return info, ErrIncompleteRecord
	}

	// Parse individual extensions
	for offset+4 <= extensionsEnd {
		extType := binary.BigEndian.Uint16(hs[offset : offset+2])
		extLen := int(binary.BigEndian.Uint16(hs[offset+2 : offset+4]))
		offset += 4

		if offset+extLen > extensionsEnd {
			break
		}

		if extType == ExtensionServerName {
			// SNI Extension found!
			sniData := hs[offset : offset+extLen]
			if len(sniData) >= 5 {
				// server_name_list length (2 bytes)
				// name_type (1 byte): 0 = host_name
				nameType := sniData[2]
				if nameType == 0 {
					nameLen := int(binary.BigEndian.Uint16(sniData[3:5]))
					if len(sniData) >= 5+nameLen {
						info.SNI = string(sniData[5 : 5+nameLen])
						// Calculate offset in original data slice
						// Original data has 5-byte record header, then hs
						info.SNIOffset = 5 + offset + 5
						info.SNILength = nameLen
						return info, nil
					}
				}
			}
		}

		offset += extLen
	}

	return info, ErrSNINotFound
}

