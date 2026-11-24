package tlshelper

import (
	"testing"
)

func TestGenerateAndParseClientHello(t *testing.T) {
	testDomains := []string{
		"speedtest.net",
		"cloudflare.com",
		"varzesh3.com",
		"my-very-long-subdomain.test.example.org",
	}

	for _, domain := range testDomains {
		raw := GenerateClientHello(domain)
		if !IsTLSHandshake(raw) {
			t.Fatalf("expected IsTLSHandshake=true for domain %s", domain)
		}

		info, err := ParseClientHello(raw)
		if err != nil {
			t.Fatalf("ParseClientHello failed for %s: %v", domain, err)
		}

		if info.SNI != domain {
			t.Fatalf("expected SNI %q, got %q", domain, info.SNI)
		}

		if info.SNIOffset <= 0 || info.SNILength != len(domain) {
			t.Fatalf("invalid SNIOffset (%d) or SNILength (%d) for %s", info.SNIOffset, info.SNILength, domain)
		}

		extracted := string(raw[info.SNIOffset : info.SNIOffset+info.SNILength])
		if extracted != domain {
			t.Fatalf("extracted SNI slice %q != expected %q", extracted, domain)
		}
	}
}

func TestParseInvalidData(t *testing.T) {
	_, err := ParseClientHello([]byte{0x00, 0x01, 0x02})
	if err == nil {
		t.Fatal("expected error on truncated data, got nil")
	}

	httpData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	if IsTLSHandshake(httpData) {
		t.Fatal("expected IsTLSHandshake=false for HTTP request")
	}

	_, err = ParseClientHello(httpData)
	if err != ErrNotTLS {
		t.Fatalf("expected ErrNotTLS, got %v", err)
	}
}

