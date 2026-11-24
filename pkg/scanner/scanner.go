package scanner

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"
)

// Result represents a single scan result for a candidate SNI domain
type Result struct {
	Domain   string
	Success  bool
	Latency  time.Duration
	Error    error
}

// Scanner performs concurrent fake SNI domain discovery
type Scanner struct {
	targetIP   string
	targetPort string
	timeout    time.Duration
	verbose    bool
}

// NewScanner creates a new SNI scanner instance
func NewScanner(targetIP, targetPort string, timeout time.Duration, verbose bool) *Scanner {
	return &Scanner{
		targetIP:   targetIP,
		targetPort: targetPort,
		timeout:    timeout,
		verbose:    verbose,
	}
}

// DefaultCandidates returns a curated list of commonly whitelisted domains
func DefaultCandidates() []string {
	return []string{
		// Cloud & CDN providers
		"cloudflare.com",
		"cdn.cloudflare.net",
		"aws.amazon.com",
		"azure.microsoft.com",
		"cloud.google.com",
		
		// Speed test sites
		"speedtest.net",
		"fast.com",
		"speed.cloudflare.com",
		
		// Major tech companies
		"google.com",
		"www.google.com",
		"youtube.com",
		"microsoft.com",
		"apple.com",
		"amazon.com",
		"facebook.com",
		
		// Developer tools
		"github.com",
		"gitlab.com",
		"npmjs.com",
		"pypi.org",
		
		// Security & DNS
		"1.1.1.1",
		"8.8.8.8",
		"quad9.net",
		"opendns.com",
		
		// CDN services
		"fastly.com",
		"akamai.com",
		"cloudfront.net",
		
		// Popular sites
		"wikipedia.org",
		"reddit.com",
		"twitter.com",
		
		// Regional (Iran)
		"aparat.com",
		"digikala.com",
		"snapp.ir",
		"varzesh3.com",
		"namava.ir",
		
		// Regional (Middle East)
		"souq.com",
		"noon.com",
		
		// Misc
		"vercel.com",
		"security.vercel.com",
		"netlify.com",
	}
}

// Scan tests multiple candidate SNI domains concurrently
func (s *Scanner) Scan(ctx context.Context, candidates []string, concurrency int) []Result {
	if concurrency <= 0 {
		concurrency = 10
	}

	results := make([]Result, 0, len(candidates))
	resultsMu := sync.Mutex{}
	
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, domain := range candidates {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			
			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			result := s.testDomain(ctx, d)
			
			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()
			
			if s.verbose || result.Success {
				status := "❌"
				if result.Success {
					status = "✅"
				}
				fmt.Printf("%s %s (%.0fms)\n", status, d, result.Latency.Seconds()*1000)
			}
		}(domain)
	}

	wg.Wait()
	return results
}

// testDomain attempts a TLS connection with a specific SNI
func (s *Scanner) testDomain(ctx context.Context, sni string) Result {
	start := time.Now()
	
	dialer := &net.Dialer{
		Timeout: s.timeout,
	}
	
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.targetIP, s.targetPort))
	if err != nil {
		return Result{
			Domain:  sni,
			Success: false,
			Latency: time.Since(start),
			Error:   err,
		}
	}
	defer conn.Close()

	// Perform TLS handshake with the candidate SNI
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})

	// Set handshake deadline
	handshakeDeadline := time.Now().Add(s.timeout)
	tlsConn.SetDeadline(handshakeDeadline)

	err = tlsConn.Handshake()
	latency := time.Since(start)

	if err != nil {
		return Result{
			Domain:  sni,
			Success: false,
			Latency: latency,
			Error:   err,
		}
	}

	return Result{
		Domain:  sni,
		Success: true,
		Latency: latency,
		Error:   nil,
	}
}

// FilterSuccessful returns only successful scan results
func FilterSuccessful(results []Result) []Result {
	var successful []Result
	for _, r := range results {
		if r.Success {
			successful = append(successful, r)
		}
	}
	return successful
}

// SortByLatency sorts results by ascending latency
func SortByLatency(results []Result) []Result {
	sorted := make([]Result, len(results))
	copy(sorted, results)
	
	// Simple bubble sort (fine for small lists)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Latency > sorted[j].Latency {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	return sorted
}
