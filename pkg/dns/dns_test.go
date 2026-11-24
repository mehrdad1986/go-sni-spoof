package dns

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDNSCache(t *testing.T) {
	res := NewResolver("")

	// Add manual entry to cache
	testIP := net.ParseIP("93.184.216.34")
	res.cache["example.com"] = CacheEntry{
		IPs:       []net.IP{testIP},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	ip, err := res.Resolve(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("failed to resolve from cache: %v", err)
	}

	if !ip.Equal(testIP) {
		t.Fatalf("expected IP %s, got %s", testIP, ip)
	}
}

func TestDirectIPResolution(t *testing.T) {
	res := NewResolver("")
	direct := "8.8.8.8"
	ip, err := res.Resolve(context.Background(), direct)
	if err != nil {
		t.Fatalf("failed to resolve direct IP: %v", err)
	}
	if ip.String() != direct {
		t.Fatalf("expected direct IP %s, got %s", direct, ip)
	}
}

