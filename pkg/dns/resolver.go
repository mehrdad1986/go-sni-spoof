package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CacheEntry stores an IP address and its expiration time.
type CacheEntry struct {
	IPs       []net.IP
	ExpiresAt time.Time
}

// Resolver provides DNS-over-HTTPS (DoH) and clean DNS resolution with caching.
type Resolver struct {
	dohEndpoint string
	httpClient  *http.Client
	cache       map[string]CacheEntry
	mu          sync.RWMutex
	bootstrapIP string
}

// DoHResponse represents a standard DNS-over-HTTPS JSON response (Cloudflare/Google format).
type DoHResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// NewResolver initializes a secure DNS resolver.
func NewResolver(dohEndpoint string) *Resolver {
	if dohEndpoint == "" {
		dohEndpoint = "https://cloudflare-dns.com/dns-query"
	}

	// Bootstrap direct IP for DoH host to avoid recursive DNS lookups
	bootstrap := "1.1.1.1"
	if strings.Contains(dohEndpoint, "google") {
		bootstrap = "8.8.8.8"
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Redirect DoH domain to bootstrap IP
			host, port, _ := net.SplitHostPort(addr)
			if host == "cloudflare-dns.com" || host == "one.one.one.one" {
				addr = net.JoinHostPort(bootstrap, port)
			} else if host == "dns.google" {
				addr = net.JoinHostPort("8.8.8.8", port)
			}
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	return &Resolver{
		dohEndpoint: dohEndpoint,
		httpClient:  &http.Client{Transport: transport, Timeout: 6 * time.Second},
		cache:       make(map[string]CacheEntry),
		bootstrapIP: bootstrap,
	}
}

// Resolve returns an IPv4/IPv6 address for the given hostname.
func (r *Resolver) Resolve(ctx context.Context, host string) (net.IP, error) {
	// If it's already an IP, return directly
	if ip := net.ParseIP(host); ip != nil {
		return ip, nil
	}

	host = strings.TrimSuffix(strings.ToLower(host), ".")

	// Check cache
	r.mu.RLock()
	entry, found := r.cache[host]
	r.mu.RUnlock()

	if found && time.Now().Before(entry.ExpiresAt) && len(entry.IPs) > 0 {
		return entry.IPs[0], nil
	}

	// Fetch via DoH
	ips, ttl, err := r.queryDoH(ctx, host)
	if err == nil && len(ips) > 0 {
		r.mu.Lock()
		r.cache[host] = CacheEntry{
			IPs:       ips,
			ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
		}
		r.mu.Unlock()
		return ips[0], nil
	}

	// Fallback to direct clean UDP DNS (1.1.1.1:53 or 8.8.8.8:53)
	ips, err = r.fallbackCleanDNS(host)
	if err == nil && len(ips) > 0 {
		r.mu.Lock()
		r.cache[host] = CacheEntry{
			IPs:       ips,
			ExpiresAt: time.Now().Add(300 * time.Second),
		}
		r.mu.Unlock()
		return ips[0], nil
	}

	return nil, fmt.Errorf("failed to resolve host %q: %w", host, err)
}

func (r *Resolver) queryDoH(ctx context.Context, host string) ([]net.IP, int, error) {
	reqURL := fmt.Sprintf("%s?name=%s&type=A", r.dohEndpoint, url.QueryEscape(host))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("DoH server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var dohResp DoHResponse
	if err := json.Unmarshal(body, &dohResp); err != nil {
		return nil, 0, err
	}

	var ips []net.IP
	minTTL := 300
	for _, ans := range dohResp.Answer {
		if ans.Type == 1 { // Type A (IPv4)
			ip := net.ParseIP(ans.Data)
			if ip != nil {
				ips = append(ips, ip)
				if ans.TTL > 0 && ans.TTL < minTTL {
					minTTL = ans.TTL
				}
			}
		}
	}

	if len(ips) == 0 {
		return nil, 0, fmt.Errorf("no A records returned for %s", host)
	}

	return ips, minTTL, nil
}

func (r *Resolver) fallbackCleanDNS(host string) ([]net.IP, error) {
	cleanResolvers := []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}
	var lastErr error

	for _, srv := range cleanResolvers {
		d := net.Dialer{Timeout: 3 * time.Second}
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return d.DialContext(ctx, "udp", srv)
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		ips, err := resolver.LookupIP(ctx, "ip4", host)
		cancel()

		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

