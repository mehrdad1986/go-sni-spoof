package spoof

import (
	"bytes"
	"testing"

	"go-sni-spoof/pkg/stats"
	"go-sni-spoof/pkg/tlshelper"
)

func TestSplitIntoTLSRecords(t *testing.T) {
	domain := "speedtest.net"
	rawClientHello := tlshelper.GenerateClientHello(domain)

	// Split into multi-record TLS
	records, err := SplitIntoTLSRecords(rawClientHello, 10)
	if err != nil {
		t.Fatalf("SplitIntoTLSRecords failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// Verify Record 1
	if records[0][0] != tlshelper.RecordTypeHandshake {
		t.Fatalf("record 1 type not handshake: %d", records[0][0])
	}

	// Verify Record 2
	if records[1][0] != tlshelper.RecordTypeHandshake {
		t.Fatalf("record 2 type not handshake: %d", records[1][0])
	}

	// Recombine handshake payloads and verify
	recombinedPayload := append(records[0][5:], records[1][5:]...)
	originalPayload := rawClientHello[5:]

	if !bytes.Equal(recombinedPayload, originalPayload) {
		t.Fatal("recombined TLS record payloads do not match original ClientHello")
	}
}

func TestDecoyManagerRotation(t *testing.T) {
	list := []string{"siteA.com", "siteB.com", "siteC.com"}
	dm := NewDecoyManager("siteA.com", list, stats.DefaultMetrics)

	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		sni := dm.GetDecoySNI()
		seen[sni] = true
	}

	if len(seen) < 2 {
		t.Fatalf("expected decoy rotation, but only saw: %v", seen)
	}

	payload := dm.GenerateDecoyPayload()
	if !tlshelper.IsTLSHandshake(payload) {
		t.Fatal("GenerateDecoyPayload did not produce valid TLS Handshake")
	}
}

func TestHTTPObfuscation(t *testing.T) {
	req := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nAccept: */*\r\n\r\n")
	if !IsHTTPRequest(req) {
		t.Fatal("expected IsHTTPRequest=true")
	}

	obf := ObfuscateHostHeader(req)
	if !bytes.Contains(obf, []byte("hOsT: example.com")) {
		t.Fatalf("expected obfuscated header, got:\n%s", string(obf))
	}
}

