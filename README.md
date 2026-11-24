# Go SNI Spoofing (DPI Bypass Engine)

> High-performance, cross-platform Deep Packet Inspection (DPI) bypass proxy and SNI spoofing engine written in 100% pure Go (Zero CGO, zero external dependencies).
>
> Re-engineered from legacy Python implementations to eliminate critical bugs, stream instability, platform lock-in, and driver dependency crashes.

---

## 📑 Table of Contents
1. [Overview & Project Background](#overview--project-background)
2. [Bugs Found in Original Python Project & How They Are Fixed](#bugs-found-in-original-python-project--how-they-are-fixed)
3. [DPI Evasion Strategies](#dpi-evasion-strategies)
4. [Operating Modes](#operating-modes)
5. [Architecture & Flow](#architecture--flow)
6. [Quick Start & Usage](#quick-start--usage)
7. [Command-Line Flags & Configuration](#command-line-flags--configuration)
8. [Cross-Platform Binaries & Build Guide](#cross-platform-binaries--build-guide)

---

## Overview & Project Background

Deep Packet Inspection (DPI) middleboxes deployed by restrictive ISPs inspect the plaintext **Server Name Indication (SNI)** field located inside the initial TLS `ClientHello` packet during an HTTPS handshake. If the SNI matches a blocked domain (such as YouTube, Twitter, Instagram, or VPN endpoints), the firewall terminates the TCP stream with spoofed `TCP RST` packets or drops subsequent segments.

Original Python SNI-spoofing tools pioneered a technique to inject a fake TLS `ClientHello` with a whitelisted/decoy SNI using manipulated TCP sequence numbers or raw packets. While innovative, previous Python implementations suffered from severe architectural limitations, network instability on fiber networks, high CPU usage, and Windows-only driver lock-in.

This Go rewrite (**`go-sni-spoof`**) completely modernizes the concept into an unprivileged, rock-solid, multi-protocol proxy engine that runs natively on **macOS (Darwin Apple Silicon & Intel)**, **Linux**, and **Windows**.

---

## Bugs Found in Original Python Project & How They Are Fixed

| # | Bug / Issue in Legacy Python Tool | Severity | Root Cause in Python | Resolution in this Go Engine |
|---|------------------------------------------|----------|----------------------|------------------------------|
| **1** | **Stream breakdown on fast / fiber connections** (*Issue #37*) | **Critical** | Unsynchronized raw socket writes and arbitrary sleeps caused TCP sequence mismatch, duplicate ACKs, ACK storms, and sudden `RST` resets from upstream servers on low-latency fiber links. | **Strict TCP sequence alignment and RFC-compliant TLS Multi-Record segmentation**. Eliminates race conditions and prevents server-side drops entirely. |
| **2** | **Corrupted / truncated TLS 1.3 ClientHello parsing** | **High** | Fixed-size buffer reads (`sock.recv(4096)`) broke when modern TLS 1.3 ClientHellos (with large cipher lists, ALPN, Kyber/post-quantum key shares) arrived fragmented across multiple TCP packets. | **Streaming TLS Record Assembler**: Reads and validates TLS record layer length and handshake length before parsing, handling multi-packet fragmentation seamlessly. |
| **3** | **Platform lock-in & Windows driver crashes** (*Issues #2, #3, #38*) | **Critical** | Bound to Windows `WinDivert` kernel drivers (`WinDivert.dll`). Produced fatal `OSError: [WinError 6] The handle is invalid` on task cancellation and could not run on macOS or Linux. | **100% Pure Go (Zero CGO)**. Uses standard user-space socket options (`IP_TTL`, TCP_NODELAY, TLS record splitting). Runs natively on macOS, Linux, and Windows without root or kernel drivers. |
| **4** | **Memory leaks & File Descriptor exhaustion** (*Issue #20*) | **High** | Python event loop did not cleanly terminate broken connections or half-closed TCP streams, causing socket leaks in `CLOSE_WAIT` and abrupt crashes under load. | **Goroutine pool with `sync.Pool` zero-allocation buffers**, context cancellation, strict read/write deadlines, and bi-directional TCP half-close (`CloseWrite()`) propagation. |
| **5** | **Static / predictable decoy SNIs & JA3 fingerprinting** | **Medium** | Used hardcoded decoy domains and static TLS payloads easily identified and blacklisted by heuristic DPI engines. | **Dynamic Decoy SNI Rotation** with randomized extensions, modern browser cipher suites (Chrome/Firefox), and customizable domain pools. |
| **6** | **Single Point of Failure (Only one evasion technique)** | **High** | Relied purely on WinDivert sequence manipulation. If an ISP middlebox reassembled out-of-order packets or dropped bad sequences, the bypass failed completely. | **Multi-Strategy Evasion Engine**: Supports TLS record splitting, TCP segmentation, low-TTL packet injection, HTTP header obfuscation, and hybrid combo mode. |
| **7** | **DNS Leaks & ISP DNS Poisoning** | **High** | Used system DNS resolution (`socket.gethostbyname`), which ISPs in censored regions poison by returning fake IP addresses (e.g. `10.10.34.34`). | **Built-in Encrypted DNS (DoH)**: Queries Cloudflare / Google DNS over HTTPS directly via bootstrap IPs, with in-memory TTL caching to prevent DNS leaks and poisoning. |
| **8** | **Rigid Proxy Integration (No SOCKS5 or HTTP CONNECT)** | **Medium** | Original project only provided raw port mapping, forcing users to configure complex external routing tools (like Proxifier). | **Multi-Protocol Listener ("Mixed" mode)**: Single listening port automatically multiplexes SOCKS5 (RFC 1928), HTTP CONNECT, and direct TCP tunnels. |
| **9** | **Abrupt process termination without diagnostics** | **Medium** | Unhandled exceptions crashed the script silently without giving user actionable connection diagnostics. | **Preflight Test Mode (`-test`)**: Built-in diagnostic utility to test and verify DPI bypass capability against any target server before starting proxy. |

---

## DPI Evasion Strategies

The engine offers 4 evasion strategies configured via `-strategy`:

### 1. `combo` (Default & Recommended)
Combines **Low-TTL Decoy Injection** with **TLS Record Splitting**:
1. Injects a realistic decoy TLS `ClientHello` (with whitelisted SNI like `speedtest.net`) configured with a low IP Time-to-Live (`TTL=5`).
2. The decoy packet reaches the ISP's DPI middlebox (2–5 hops away). The DPI registers the connection as whitelisted traffic and disables deep inspection for that flow.
3. The decoy packet expires before reaching the destination server, ensuring the target server never receives invalid data.
4. The genuine TLS `ClientHello` is transmitted split across two TLS records (or TCP segments). The real server receives and reassembles it cleanly.

### 2. `fragment`
Splits the TLS `ClientHello` into multiple TLS records according to RFC 5246 / RFC 8446. Commercial DPI engines (Fortinet, Sandvine, Huawei, etc.) do not reassemble multi-record ClientHellos to prevent memory exhaustion, while standard web servers (Cloudflare, Nginx, Envoy) seamlessly process them.

### 3. `decoy`
Injects a fake `ClientHello` containing a rotating decoy SNI domain, then transmits the real handshake payload with micro-pacing.

### 4. `ttl`
Transmits a decoy packet with low TTL (`IP_TTL`), followed by immediate restoration of normal socket TTL for the real handshake.

---

## Operating Modes

Configured via `-mode`:

* **`mixed` (Default)**: Automatically detects incoming protocol:
  * If client speaks SOCKS5 (`0x05`) &rarr; processes via SOCKS5 handler.
  * If client sends HTTP `CONNECT` &rarr; processes via HTTP CONNECT proxy.
  * If client connects directly with target specified &rarr; operates as direct TCP tunnel.
* **`socks5`**: Standard RFC 1928 SOCKS5 proxy server. Works with browsers, Telegram, Clash, v2ray/Xray, curl (`--socks5`), etc.
* **`http`**: Standard HTTP/HTTPS proxy server. Works with OS proxy settings, browsers, curl (`-x`), etc.
* **`tunnel`**: Direct port forwarder (e.g. `-listen 127.0.0.1:8443 -connect 104.16.123.96:443`).

---

## Architecture & Flow

```
+--------------------------------------------------------------------------+
|                               Client App                                 |
|               (Browser / Telegram / Curl / v2ray / System)               |
+--------------------------------------------------------------------------+
                                     |
                                     v [Local Connection]
+--------------------------------------------------------------------------+
|                     Go SNI Spoofing Proxy Listener                       |
|                          (127.0.0.1:8080)                                |
|  - Protocol Auto-Detector (SOCKS5 / HTTP CONNECT / Direct Tunnel)        |
|  - Clean DNS Resolver (DoH via Cloudflare / Google with In-Memory Cache) |
+--------------------------------------------------------------------------+
                                     |
                                     v [Intercept Handshake]
+--------------------------------------------------------------------------+
|                        DPI Evasion Engine                                |
|  1. Synthesize Decoy TLS ClientHello (Decoy SNI: speedtest.net)          |
|  2. Inject Decoy with Low TTL (poisons ISP DPI flow cache)               |
|  3. Fragment Real TLS ClientHello across RFC-compliant TLS Records       |
+--------------------------------------------------------------------------+
            |                                            |
            v [Decoy Packet: Dies at hop 5]              v [Real Handshake]
+---------------------------------------+   +------------------------------+
|           ISP DPI Middlebox           |   |      Destination Server      |
|  - Sees Decoy SNI: speedtest.net      |   |  - Reassembles TLS records   |
|  - Marks flow as "Whitelisted"        |   |  - Completes TLS Handshake   |
|  - Passes subsequent traffic          |   |  - Sends TLS ServerHello     |
+---------------------------------------+   +------------------------------+
```

---

## Quick Start & Usage

### 1. Run Preflight Test
Before running the proxy, verify that your network connection and decoy SNI can successfully bypass DPI:

```bash
# Test against YouTube or Cloudflare target
./sni-spoof -test www.youtube.com:443 -strategy combo -fake-sni speedtest.net
```

### 2. Start Proxy Server
```bash
# Run in mixed mode (SOCKS5 + HTTP CONNECT) on port 8080
./sni-spoof -listen 127.0.0.1:8080
```

### 3. Test with Curl
```bash
# Test through SOCKS5 proxy
curl -x socks5h://127.0.0.1:8080 -I https://www.youtube.com

# Test through HTTP CONNECT proxy
curl -x http://127.0.0.1:8080 -I https://www.youtube.com
```

### 4. Direct TCP Port Tunnel Mode
If you want to forward a specific local port to an upstream server (e.g. Cloudflare clean IP):
```bash
./sni-spoof -listen 127.0.0.1:8443 -connect 104.16.123.96:443 -fake-sni speedtest.net
```

---

## Command-Line Flags & Configuration

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-listen` | string | `127.0.0.1:8080` | Local address to listen on |
| `-connect` | string | `""` | Upstream target address for direct tunnel mode |
| `-mode` | string | `mixed` | Operating mode: `mixed`, `socks5`, `http`, `tunnel` |
| `-strategy` | string | `combo` | Evasion strategy: `combo`, `fragment`, `decoy`, `ttl` |
| `-fake-sni` | string | `speedtest.net` | Primary decoy SNI domain |
| `-fake-sni-list`| string | `""` | Comma-separated list of decoy SNIs for rotation |
| `-frag-size` | int | `3` | Byte offset to split TLS ClientHello |
| `-frag-delay` | int | `5` | Pacing delay between fragments in milliseconds |
| `-ttl` | int | `5` | TTL hop count for decoy packets in TTL mode |
| `-doh` | string | Cloudflare DoH | DNS-over-HTTPS endpoint for clean resolution |
| `-clean-dns` | bool | `true` | Enable built-in clean DNS resolver |
| `-test` | string | `""` | Target to run preflight bypass test against |
| `-verbose` | bool | `false` | Enable verbose debug output |
| `-config` | string | `""` | Path to JSON configuration file |

---

## Cross-Platform Binaries & Build Guide

Pre-compiled standalone binaries are located in the `bin/` directory:

| Platform | Binary Path |
|----------|-------------|
| **macOS (Apple Silicon M1/M2/M3)** | `bin/sni-spoof-darwin-arm64` |
| **macOS (Intel Core)** | `bin/sni-spoof-darwin-amd64` |
| **Linux (64-bit AMD/Intel)** | `bin/sni-spoof-linux-amd64` |
| **Linux (ARM 64-bit / Raspberry Pi)** | `bin/sni-spoof-linux-arm64` |
| **Windows (64-bit)** | `bin/sni-spoof-windows-amd64.exe` |

### Building from Source
```bash
# Build native binary
make build

# Run automated unit tests
make test

# Cross-compile for all operating systems
make cross-compile
```

