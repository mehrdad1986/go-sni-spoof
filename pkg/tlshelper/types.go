package tlshelper

// TLS Content Types (RFC 5246 / RFC 8446)
const (
	RecordTypeChangeCipherSpec byte = 20
	RecordTypeAlert            byte = 21
	RecordTypeHandshake         byte = 22
	RecordTypeApplicationData  byte = 23
)

// TLS Handshake Types
const (
	HandshakeTypeClientHello byte = 1
	HandshakeTypeServerHello byte = 2
)

// TLS Extension Types
const (
	ExtensionServerName           uint16 = 0x0000
	ExtensionSupportedGroups      uint16 = 0x000a
	ExtensionSessionTicket        uint16 = 0x0023
	ExtensionApplicationLayerProt uint16 = 0x0010 // ALPN
	ExtensionSignatureAlgorithms  uint16 = 0x000d
	ExtensionSupportedVersions    uint16 = 0x002b
	ExtensionKeyShare             uint16 = 0x0033
)

// TLS Versions
const (
	VersionTLS10 uint16 = 0x0301
	VersionTLS11 uint16 = 0x0302
	VersionTLS12 uint16 = 0x0303
	VersionTLS13 uint16 = 0x0304
)

// ClientHelloInfo holds parsed metadata from a TLS ClientHello
type ClientHelloInfo struct {
	SNI             string
	RawPayload      []byte
	RecordLength    int
	HandshakeLength int
	SNIOffset       int
	SNILength       int
	IsComplete      bool
}

