package tlshelper

import (
	"crypto/rand"
	"encoding/binary"
)

// GenerateClientHello builds a realistic, valid TLS 1.3/1.2 ClientHello
// with the provided decoy SNI domain name.
func GenerateClientHello(sni string) []byte {
	var body []byte

	// 1. Client Version: TLS 1.2 (0x0303) as legacy version
	body = append(body, 0x03, 0x03)

	// 2. Client Random: 32 random bytes
	randomBytes := make([]byte, 32)
	_, _ = rand.Read(randomBytes)
	body = append(body, randomBytes...)

	// 3. Session ID: 32 random bytes (common in TLS 1.3 for middlebox compatibility)
	sessionID := make([]byte, 32)
	_, _ = rand.Read(sessionID)
	body = append(body, byte(len(sessionID)))
	body = append(body, sessionID...)

	// 4. Cipher Suites (Modern Chrome / Firefox cipher suites)
	cipherSuites := []uint16{
		0x1301, // TLS_AES_128_GCM_SHA256
		0x1302, // TLS_AES_256_GCM_SHA384
		0x1303, // TLS_CHACHA20_POLY1305_SHA256
		0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
		0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
		0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
		0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
		0xcca9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
		0xcca8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	}
	// Cipher suites length field is 2 bytes (big-endian), not 1
	csLen := make([]byte, 2)
	binary.BigEndian.PutUint16(csLen, uint16(len(cipherSuites)*2))
	body = append(body, csLen...)
	for _, cs := range cipherSuites {
		csBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(csBytes, cs)
		body = append(body, csBytes...)
	}

	// 5. Compression Methods: 1 byte length (1), 0x00 (null compression)
	body = append(body, 0x01, 0x00)

	// 6. Extensions
	var extensions []byte

	// 6a. SNI Extension
	if sni != "" {
		sniBytes := []byte(sni)
		nameLen := len(sniBytes)
		listLen := nameLen + 3
		extLen := listLen + 2

		ext := make([]byte, 4+extLen)
		binary.BigEndian.PutUint16(ext[0:2], ExtensionServerName)
		binary.BigEndian.PutUint16(ext[2:4], uint16(extLen))
		binary.BigEndian.PutUint16(ext[4:6], uint16(listLen))
		ext[6] = 0 // host_name
		binary.BigEndian.PutUint16(ext[7:9], uint16(nameLen))
		copy(ext[9:], sniBytes)

		extensions = append(extensions, ext...)
	}

	// 6b. Supported Versions Extension (TLS 1.3, TLS 1.2)
	supportedVers := []byte{
		0x00, 0x2b, // Extension Supported Versions
		0x00, 0x05, // Length
		0x04,       // List length (4 bytes)
		0x03, 0x04, // TLS 1.3
		0x03, 0x03, // TLS 1.2
	}
	extensions = append(extensions, supportedVers...)

	// 6c. Supported Groups Extension (X25519, secp256r1)
	supportedGroups := []byte{
		0x00, 0x0a, // Supported Groups
		0x00, 0x06, // Length
		0x00, 0x04, // List length
		0x00, 0x1d, // X25519
		0x00, 0x17, // secp256r1
	}
	extensions = append(extensions, supportedGroups...)

	// 6d. ALPN Extension (h2, http/1.1)
	alpnExt := []byte{
		0x00, 0x10, // ALPN
		0x00, 0x0e, // Length: 14
		0x00, 0x0c, // List length: 12
		0x02, 'h', '2',
		0x08, 'h', 't', 't', 'p', '/', '1', '.', '1',
	}
	extensions = append(extensions, alpnExt...)

	// Append extensions to body
	extLenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(extLenBytes, uint16(len(extensions)))
	body = append(body, extLenBytes...)
	body = append(body, extensions...)

	// 7. Handshake header: Type (1 byte) + Length (3 bytes uint24)
	hsHeader := make([]byte, 4)
	hsHeader[0] = HandshakeTypeClientHello
	hsLen := len(body)
	hsHeader[1] = byte(hsLen >> 16)
	hsHeader[2] = byte(hsLen >> 8)
	hsHeader[3] = byte(hsLen)

	fullHandshake := append(hsHeader, body...)

	// 8. TLS Record Header: 0x16 (Handshake), 0x0301 (Legacy TLS 1.0 record version), Length (2 bytes)
	recordHeader := make([]byte, 5)
	recordHeader[0] = RecordTypeHandshake
	recordHeader[1] = 0x03
	recordHeader[2] = 0x01
	binary.BigEndian.PutUint16(recordHeader[3:5], uint16(len(fullHandshake)))

	return append(recordHeader, fullHandshake...)
}

