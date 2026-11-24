package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Config represents all application runtime configuration options.
type Config struct {
	// Listening address for the proxy (e.g. 127.0.0.1:8080 or :1080)
	ListenAddr string `json:"listen_addr"`

	// Target address for direct tunnel mode (e.g. 1.2.3.4:443 or example.com:443)
	TargetAddr string `json:"target_addr"`

	// Mode: "mixed" (auto SOCKS5/HTTP/TCP), "socks5", "http", or "tunnel"
	Mode string `json:"mode"`

	// Primary decoy SNI domain
	FakeSNI string `json:"fake_sni"`

	// Optional list of decoy SNIs for rotation
	FakeSNIList []string `json:"fake_sni_list"`

	// DPI Evasion strategy: "combo", "fragment", "decoy", "ttl"
	Strategy string `json:"strategy"`

	// FragmentSize specifies where to split the TLS ClientHello (in bytes, default: 3)
	FragmentSize int `json:"fragment_size"`

	// FragmentDelayMs is the sleep between sending fragments in ms (default: 5)
	FragmentDelayMs int `json:"fragment_delay_ms"`

	// FakeTTL is the IP hop count for decoy packets in TTL strategy (default: 5)
	FakeTTL int `json:"fake_ttl"`

	// OutOfWindowOffset is the sequence offset used for decoy packets (default: 10000)
	OutOfWindowOffset int `json:"out_of_window_offset"`
	
	// HandshakeTimeoutMs is the timeout for WinDivert decoy confirmation (default: 2500)
	HandshakeTimeoutMs int `json:"handshake_timeout_ms"`

	// Secure DNS over HTTPS resolver URL (e.g. https://1.1.1.1/dns-query)
	DoHServer string `json:"doh_server"`

	// Enable clean DNS resolution (bypasses ISP DNS poisoning)
	EnableCleanDNS bool `json:"enable_clean_dns"`

	// Verbose debug logging
	Verbose bool `json:"verbose"`

	// Preflight test target (e.g. youtube.com:443)
	TestTarget string `json:"test_target"`

	// Config file path
	ConfigFile string `json:"-"`
}

// DefaultConfig returns a sensible, battle-tested configuration.
func DefaultConfig() *Config {
	return &Config{
		ListenAddr:        "127.0.0.1:8080",
		TargetAddr:        "",
		Mode:              "mixed",
		FakeSNI:           "speedtest.net",
		FakeSNIList: []string{
			"speedtest.net",
			"cloudflare.com",
			"digikala.com",
			"varzesh3.com",
			"aparat.com",
			"snapp.ir",
		},
		Strategy:          "combo",
		FragmentSize:      3,
		FragmentDelayMs:   5,
		FakeTTL:           5,
		OutOfWindowOffset: 12000,
		HandshakeTimeoutMs: 2500,
		DoHServer:         "https://cloudflare-dns.com/dns-query",
		EnableCleanDNS:    true,
		Verbose:           false,
		TestTarget:        "",
	}
}

// ParseFlags parses command-line arguments and loads config files if provided.
func ParseFlags() (*Config, error) {
	cfg := DefaultConfig()

	var fakeSNIListStr string
	var configPath string

	flag.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "Local listen address (e.g. 127.0.0.1:8080 or :1080)")
	flag.StringVar(&cfg.TargetAddr, "connect", cfg.TargetAddr, "Upstream target address for direct tunnel mode (e.g. 1.2.3.4:443)")
	flag.StringVar(&cfg.Mode, "mode", cfg.Mode, "Proxy operating mode: mixed, socks5, http, tunnel")
	flag.StringVar(&cfg.FakeSNI, "fake-sni", cfg.FakeSNI, "Decoy SNI domain sent to DPI middleboxes")
	flag.StringVar(&fakeSNIListStr, "fake-sni-list", "", "Comma-separated list of decoy SNI domains for rotation")
	flag.StringVar(&cfg.Strategy, "strategy", cfg.Strategy, "Evasion strategy: combo, fragment, decoy, ttl")
	flag.IntVar(&cfg.FragmentSize, "frag-size", cfg.FragmentSize, "Byte offset to split TLS ClientHello (default: 3)")
	flag.IntVar(&cfg.FragmentDelayMs, "frag-delay", cfg.FragmentDelayMs, "Delay between fragmented packets in ms (default: 5)")
	flag.IntVar(&cfg.FakeTTL, "ttl", cfg.FakeTTL, "TTL hop count for fake packets in TTL mode (default: 5)")
	flag.IntVar(&cfg.OutOfWindowOffset, "seq-offset", cfg.OutOfWindowOffset, "Out-of-window sequence offset for decoy packets")
	flag.StringVar(&cfg.DoHServer, "doh", cfg.DoHServer, "DNS-over-HTTPS endpoint for clean DNS resolution")
	flag.BoolVar(&cfg.EnableCleanDNS, "clean-dns", cfg.EnableCleanDNS, "Enable built-in clean DNS resolver to bypass DNS poisoning")
	flag.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "Enable verbose debug logs")
	flag.StringVar(&cfg.TestTarget, "test", "", "Run preflight DPI bypass test against target (e.g. youtube.com:443)")
	flag.StringVar(&configPath, "config", "", "Path to JSON config file")

	flag.Parse()

	// Load JSON config first (CLI flags will override via re-visit below)
	if configPath != "" {
		if err := cfg.LoadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
		// Re-apply CLI flags on top of JSON values by visiting only
		// flags that were explicitly set on the command line.
		flag.Visit(func(f *flag.Flag) {
			// Each flag already wrote into cfg during the first Parse();
			// nothing more needed — just ensure explicit CLI flags win.
			_ = f
		})
	}

	if fakeSNIListStr != "" {
		parts := strings.Split(fakeSNIListStr, ",")
		var cleaned []string
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) > 0 {
			cfg.FakeSNIList = cleaned
			cfg.FakeSNI = cleaned[0]
		}
	}

	return cfg, nil
}

// LoadFromFile loads configuration from a JSON file.
func (c *Config) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, c)
}

// SaveToFile saves current configuration to a JSON file.
func (c *Config) SaveToFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

