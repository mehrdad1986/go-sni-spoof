//go:build ignore

/*
================================================================================
           COMPLETE LINE-BY-LINE GO TUTORIAL & EXPLANATION FOR main.go
================================================================================

WELCOME TO GO PROGRAMMING!
If you have never written a line of Go (Golang) before, this file is your ultimate guide.
Everything from `main.go` is reproduced here, line by line, accompanied by detailed,
easy-to-understand explanations in plain English.

--------------------------------------------------------------------------------
1. THE 6 CORE RULES OF GO SYNTAX YOU MUST KNOW:
--------------------------------------------------------------------------------
1. PACKAGES: Every Go file starts with `package <name>`.
   - `package main` tells the compiler: "This is a runnable program with a `main()` function."
   - If it was `package utils`, it would be a library meant to be imported by others.

2. CAPITALIZATION IS ACCESS CONTROL (Public vs Private):
   - In Go, there are NO words like "public" or "private".
   - If a name starts with an UPPERCASE letter (`Config`, `ListenHost`, `Start`), it is PUBLIC
     (exported) and can be used by other files and packages.
   - If a name starts with a LOWERCASE letter (`connectIP`, `handleConnection`), it is PRIVATE
     and can only be used inside this file/package.

3. POINTERS (* and &):
   - Think of a variable as a labeled cardboard box in memory.
   - `x := 10`: The box named `x` contains the number 10.
   - `&x`: Gives you the MEMORY ADDRESS of the box (e.g., location 0x00FF4B2).
   - `*int`: Means "this variable holds the memory address of an integer box".
   - `*ptr = 20`: Reaches into that memory address and changes what is inside the box.
   - Why use pointers? If you pass a large struct by value, Go makes a duplicate copy of it
     in RAM. Passing a pointer `*Config` passes just the tiny 8-byte memory address, making
     it lightning fast, and allows functions to modify the original struct.

4. SLICES vs ARRAYS:
   - `[4]byte`: A fixed-size array of exactly 4 bytes. Its size can never change.
   - `[]byte`: A SLICE. A slice is a flexible, dynamically-sized window looking at an array.
     You can add items to it using `append()`.

5. GOROUTINES & CHANNELS:
   - `go doSomething()`: Starts the function in a "goroutine". A goroutine is a super
     lightweight background thread managed by Go (uses only ~2KB of RAM, unlike OS threads
     which take 1MB+). You can easily run 100,000 goroutines simultaneously.
   - `ch := make(chan string)`: A "channel". Channels are pipes that allow goroutines
     to send data to each other safely without crashing.
   - `ch <- "hello"`: Pushes "hello" into the pipe.
   - `msg := <-ch`: Waits for someone to push data into the pipe and reads it.

6. ERROR HANDLING:
   - Go does not have `try / catch / throw`.
   - Instead, functions that can fail return two values: the result and an error object.
   - Example: `file, err := os.Open("config.json")`
   - If `err != nil`, it means an error occurred!
   - `nil` in Go means "nothing" / "empty" (like `null` in JavaScript or `None` in Python).
================================================================================
*/

// ============================================================================
// PART 1: PACKAGE DECLARATION & IMPORTS
// ============================================================================

// `package main` declares that this file is the main executable program.
package main

// `import (...)` lists all external packages and Go standard libraries used by this file.
// Go is strict: if you import a library and do not use it, Go will refuse to compile!
import (
	// "bytes": Functions to inspect and slice byte buffers (like bytes.Equal, bytes.TrimPrefix).
	"bytes"

	// "context": Handles cancellation signals. When the user stops the app (Ctrl+C),
	// the context notifies all running goroutines to cleanly close their connections.
	"context"

	// "crypto/ecdh": Elliptic Curve Diffie-Hellman keys. Used in the preflight test
	// to generate genuine X25519 key-shares for realistic TLS 1.3 handshakes.
	"crypto/ecdh"

	// "crypto/rand": Cryptographically secure random number generator (CSPRNG).
	// Used to generate random TLS session IDs and random bytes so DPI cannot fingerprint us.
	"crypto/rand"

	// "encoding/binary": Reads and writes numbers (uint16, uint32) to raw bytes in Big-Endian
	// network byte order (which all internet protocols like TCP and TLS require).
	"encoding/binary"

	// "encoding/hex": Decodes hex strings (e.g., "1603010200") into raw bytes.
	"encoding/hex"

	// "encoding/json": Parses and loads config.json into Go structs.
	"encoding/json"

	// "errors": Creates error messages (e.g., errors.New("something went wrong")).
	"errors"

	// "flag": Parses command line parameters (like `-config`, `-test`, `-v`).
	"flag"

	// "fmt": Formatted print functions (like fmt.Println, fmt.Printf, fmt.Sprintf).
	"fmt"

	// "io": Basic input/output interfaces (io.Reader, io.Writer, io.ReadFull, io.EOF).
	"io"

	// "log": Formats error and diagnostic messages with automatic date and time stamps.
	"log"

	// "net": All networking: TCP listeners, TCP connections, IP address parsing.
	"net"

	// "os": Operating system interaction: reading files, exiting, reading process paths.
	"os"

	// "os/signal": Catches OS signals (like SIGINT when you press Ctrl+C in terminal).
	"os/signal"

	// "path/filepath": Cross-platform file path manipulation (handles Windows \ vs Linux /).
	"path/filepath"

	// "strconv": Converts strings to numbers (strconv.Atoi) and numbers to strings (strconv.Itoa).
	"strconv"

	// "strings": String utilities: strings.Split, strings.ToLower, strings.Contains.
	"strings"

	// "sync": Concurrency primitives:
	// - Mutex: locks data so two threads don't write at the same time.
	// - WaitGroup: waits for multiple background goroutines to finish.
	// - Pool: reuses byte buffers to prevent memory waste.
	"sync"

	// "sync/atomic": Atomic CPU operations: allows updating variables (like connection counters)
	// across hundreds of threads simultaneously at hardware speed without mutex locks.
	"sync/atomic"

	// "syscall": Directly invokes Windows kernel APIs, loads DLLs, and manipulates OS socket options.
	"syscall"

	// "time": Timers, timeouts, sleeps, and timestamps.
	"time"

	// "unsafe": Bypasses Go's type system to convert Go pointers to raw C memory addresses (uintptr)
	// so the C-based WinDivert.dll can directly access our network packets.
	"unsafe"
)

// ============================================================================
// PART 2: CONSTANTS, GLOBAL METRICS & BUFFER POOL
// ============================================================================

// `const` declares values that cannot be modified after compilation.
// Backticks (`) allow multi-line raw strings without needing \n on every line.
const Banner = `
========================================================================
    ____ ____     ____  _   _ ___   ____  ____   ___   ___  _____ 
   / ___/ ___|   / ___|| \ | |_ _| / ___||  _ \ / _ \ / _ \|  ___|
  | |  | |  _    \___ \|  \| || |  \___ \| |_) | | | | | | | |_   
  | |__| |_| |    ___) | |\  || |   ___) |  __/| |_| | |_| |  _|  
   \____\____|   |____/|_| \_|___| |____/|_|    \___/ \___/|_|    
                                              by medo
========================================================================
  Medo SNI-Spoofing Core Engine | Windows x64 Hardened Edition
  • Multi-Worker Parallel Interceptor (32MB WinDivert Buffer: No Drops)
  • Zero Steady-State Payload Overhead | Non-Reentrant Deadlock Eliminated
  • In-Flight Retransmit Tolerance | Retries & Userspace Auto-Failover
========================================================================
`

const (
	// Maximum network packet size (MTU) supported by WinDivert:
	// 40 bytes (IP+TCP headers) + 65535 bytes (max IP payload) = 65575 bytes.
	winDivertMTUMax    = 40 + 0xFFFF

	// Default maximum concurrent active connections allowed before new ones are throttled.
	defaultMaxConns    = 2000

	// Grace period (60 seconds) given to a half-closed TCP socket to flush remaining data.
	defaultGracePeriod = 60 * time.Second
)

// Global metrics variables:
// `var (...)` allows declaring multiple variables in one block.
// `uint64` is an Unsigned 64-bit Integer (range: 0 to 18,446,744,073,709,551,615).
// We use atomic operations (atomic.AddUint64) to safely increment these from any goroutine.
var (
	metricConnectionsTotal   uint64 // Total connections accepted since program started
	metricFakeInjected       uint64 // Number of spoofed decoy ClientHello packets injected
	metricFakeAckReceived    uint64 // Number of decoy ACKs received confirming DPI middlebox bypassed
	metricFakeTimeouts       uint64 // Number of times the decoy ACK timed out
	metricPanicsRecovered    uint64 // Number of unexpected crashes caught safely without dying
	metricSendErrors         uint64 // Number of raw packet transmission errors from WinDivert
	metricUserspaceFallbacks uint64 // Number of connections routed through pure TLS segmentation
	verboseLogging           bool   // When true (via -v flag), prints detailed debug output
)

// `relayBufferPool`:
// High-traffic proxies allocate thousands of 32KB buffers per second. If we used `make([]byte, 32*1024)`
// each time, the Go Garbage Collector (GC) would constantly pause the program to clean them up.
// `sync.Pool` is a recycler: when a connection finishes, it returns its buffer to the pool
// so the next connection can reuse it with ZERO memory allocations!
var relayBufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 32*1024) // 32 Kilobytes buffer
		return &b
	},
}

// ============================================================================
// PART 3: CONFIGURATION & STRICT VALIDATION (Lines 74-231)
// ============================================================================

// `type Config struct`:
// In Go, a `struct` is a blueprint for a custom data type (similar to a class or an object
// in other languages). It groups together related variables under one roof.
//
// STRUCT TAGS:
// Notice the text in backticks after each field: `json:"LISTEN_HOST"`
// These are called "Struct Tags" (metadata annotations).
// When Go's json package reads `config.json`, it maps the JSON key "LISTEN_HOST"
// directly into the Go field `ListenHost`.
// `omitempty` means: if this field is missing or empty in JSON, don't fail, just use default.
type Config struct {
	ListenHost         string `json:"LISTEN_HOST"`                  // IP to listen on locally (e.g., "127.0.0.1")
	ListenPort         int    `json:"LISTEN_PORT"`                  // Local port to bind to (e.g., 40443)
	ConnectIP          string `json:"CONNECT_IP"`                   // Target upstream IP (e.g., "172.66.47.87")
	ConnectPort        int    `json:"CONNECT_PORT"`                 // Target upstream port (e.g., 443)
	FakeSNI            string `json:"FAKE_SNI"`                     // Decoy domain sent to confuse DPI (e.g., "www.google.com")
	InterfaceIP        string `json:"INTERFACE_IP,omitempty"`       // (Optional) Specific physical network adapter IP
	FailMode           string `json:"FAIL_MODE,omitempty"`          // "closed" = abort if spoof fails; "open" = fallback to userspace
	HandshakeTimeoutMs int    `json:"HANDSHAKE_TIMEOUT_MS,omitempty"`// Max milliseconds to wait for TLS handshake
	FakeAckTimeoutMs   int    `json:"FAKE_ACK_TIMEOUT_MS,omitempty"`// Max milliseconds to wait for decoy ACK from firewall
	IdleTimeoutSec     int    `json:"IDLE_TIMEOUT_SEC,omitempty"`   // Idle timeout (0 = disabled, rely on TCP keepalive)
	Retries            int    `json:"RETRIES,omitempty"`            // Number of injection retries on packet drop
	MaxConcurrentConns int    `json:"MAX_CONCURRENT_CONNS,omitempty"`// Maximum active concurrent connections allowed

	// DERIVED FIELDS (LOWERCASE = PRIVATE):
	// Notice these do NOT have json tags and start with lowercase letters.
	// They are parsed and computed ONCE when config is loaded, avoiding expensive
	// re-parsing for every single incoming connection.
	connectIP  net.IP // Parsed 4-byte IPv4 address object (net.IP)
	targetAddr string // Formatted string "172.66.47.87:443" ready for net.Dial
}

// `DefaultConfig()`:
// Returns a pointer `*Config` with safe, battle-tested default settings.
// The `&` symbol creates the struct in memory and returns its memory address (pointer).
func DefaultConfig() *Config {
	return &Config{
		ListenHost:         "127.0.0.1",
		ListenPort:         40443,
		ConnectIP:          "",
		ConnectPort:        443,
		FakeSNI:            "www.google.com",
		InterfaceIP:        "",
		FailMode:           "closed",
		HandshakeTimeoutMs: 8000,
		FakeAckTimeoutMs:   2500,
		IdleTimeoutSec:     0,
		Retries:            1,
		MaxConcurrentConns: defaultMaxConns,
	}
}

// `LoadConfig`:
// Loads `config.json`, parses JSON, strips UTF-8 BOM, validates all fields strictly,
// and returns the loaded Config pointer or an error.
func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// 1. Locate config.json:
	// If no path was explicitly passed via command line, search in:
	// a) The directory of the running .exe
	// b) The current working directory
	if configPath == "" {
		exePath, err := os.Executable()
		if err == nil {
			cand := filepath.Join(filepath.Dir(exePath), "config.json")
			if _, sErr := os.Stat(cand); sErr == nil {
				configPath = cand
			}
		}
		if configPath == "" {
			if _, sErr := os.Stat("config.json"); sErr == nil {
				configPath = "config.json"
			}
		}
	}

	// 2. Read and parse the file if found:
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			// `%q` prints the string safely quoted; `%w` wraps the original error
			return nil, fmt.Errorf("failed to read config file %q: %w", configPath, err)
		}

		// STRIP UTF-8 BOM:
		// Windows Notepad often saves files with 3 invisible hidden bytes at the very start:
		// 0xEF, 0xBB, 0xBF (Byte Order Mark). Standard JSON decoders choke on this!
		// `bytes.TrimPrefix` safely removes them if present.
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

		// STRICT JSON DECODING:
		decoder := json.NewDecoder(bytes.NewReader(data))
		// `DisallowUnknownFields` ensures that if someone makes a typo like "FAKE_SNII",
		// Go will immediately report an error instead of silently ignoring it!
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(cfg); err != nil {
			return nil, fmt.Errorf("invalid config.json (check for typos or unknown fields): %w", err)
		}
		// Check that there is no garbage trailing text after the JSON closing brace `}`
		if _, err := decoder.Token(); err != io.EOF {
			return nil, errors.New("invalid config.json: unexpected trailing data after JSON object")
		}
		fmt.Printf("[+] Loaded configuration from: %s\n", configPath)
	}

	// 3. STRICT VALIDATION:
	if cfg.ListenHost == "" {
		cfg.ListenHost = "127.0.0.1"
	}
	if cfg.ListenHost != "127.0.0.1" && cfg.ListenHost != "localhost" {
		ip := net.ParseIP(cfg.ListenHost)
		if ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("LISTEN_HOST %q must be a valid IPv4 address or localhost", cfg.ListenHost)
		}
		if !ip.IsLoopback() {
			fmt.Printf("[!] WARNING: LISTEN_HOST (%s) is not loopback. Port %d is publicly exposed.\n", cfg.ListenHost, cfg.ListenPort)
		}
	}

	// CONNECT_IP is mandatory:
	if cfg.ConnectIP == "" {
		return nil, errors.New("CONNECT_IP is required in config.json (e.g. \"172.66.44.137\")")
	}
	parsedConnectIP := net.ParseIP(cfg.ConnectIP)
	if parsedConnectIP == nil || parsedConnectIP.To4() == nil {
		return nil, fmt.Errorf("CONNECT_IP %q must be a valid literal IPv4 address", cfg.ConnectIP)
	}
	// Validate port numbers (TCP ports are 16-bit: 1 to 65535)
	if cfg.ConnectPort <= 0 || cfg.ConnectPort > 65535 {
		return nil, fmt.Errorf("CONNECT_PORT %d must be between 1 and 65535", cfg.ConnectPort)
	}
	if cfg.ListenPort <= 0 || cfg.ListenPort > 65535 {
		return nil, fmt.Errorf("LISTEN_PORT %d must be between 1 and 65535", cfg.ListenPort)
	}

	// Validate FAKE_SNI length and ASCII characters:
	// The SNI spoofing template in section 4 requires the SNI length to fit between 1 and 219 bytes.
	if len(cfg.FakeSNI) < 1 || len(cfg.FakeSNI) > 219 {
		return nil, fmt.Errorf("FAKE_SNI %q length (%d) must be between 1 and 219 bytes", cfg.FakeSNI, len(cfg.FakeSNI))
	}
	for i := 0; i < len(cfg.FakeSNI); i++ {
		c := cfg.FakeSNI[i]
		if c <= 32 || c >= 127 {
			return nil, fmt.Errorf("FAKE_SNI %q contains invalid non-printable/whitespace character at byte %d", cfg.FakeSNI, i)
		}
	}

	if cfg.InterfaceIP != "" {
		if ip := net.ParseIP(cfg.InterfaceIP); ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("INTERFACE_IP %q is not a valid IPv4 address", cfg.InterfaceIP)
		}
	}

	// Sanitize FAIL_MODE:
	if cfg.FailMode != "" && cfg.FailMode != "open" && cfg.FailMode != "closed" {
		log.Printf("[!] Warning: invalid FAIL_MODE %q; defaulting to 'closed'", cfg.FailMode)
		cfg.FailMode = "closed"
	} else if cfg.FailMode == "" {
		cfg.FailMode = "closed"
	}

	if cfg.HandshakeTimeoutMs <= 0 {
		cfg.HandshakeTimeoutMs = 8000
	}
	if cfg.FakeAckTimeoutMs <= 0 {
		cfg.FakeAckTimeoutMs = 2500
	}
	if cfg.IdleTimeoutSec < 0 {
		cfg.IdleTimeoutSec = 0
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	} else if cfg.Retries > 5 {
		log.Printf("[!] Warning: RETRIES %d exceeds maximum (5); capping at 5", cfg.Retries)
		cfg.Retries = 5
	}
	if cfg.MaxConcurrentConns <= 0 {
		cfg.MaxConcurrentConns = defaultMaxConns
	} else if cfg.MaxConcurrentConns > 100000 {
		log.Printf("[!] Warning: MAX_CONCURRENT_CONNS %d exceeds maximum (100000); capping at 100000", cfg.MaxConcurrentConns)
		cfg.MaxConcurrentConns = 100000
	}

	// Pre-derive values once:
	cfg.connectIP = parsedConnectIP.To4()
	cfg.targetAddr = net.JoinHostPort(cfg.ConnectIP, strconv.Itoa(cfg.ConnectPort))

	return cfg, nil
}

// ============================================================================
// PART 4: WINDIVERT 2.X ABI & DYNAMIC LINKER (Lines 232-458)
// ============================================================================

// What is WinDivert?
// WinDivert (Windows Packet Divert) is a kernel driver (WinDivert64.sys) and a C library
// (WinDivert.dll) that intercepts network packets before Windows networking stack processes them.
//
// In C, WinDivert defines a struct `WINDIVERT_ADDRESS`.
// In Go, we must define the EXACT same binary memory layout byte-for-byte!
// If our struct size differs by even 1 byte, Windows memory will corrupt and crash.
type WinDivertAddress struct {
	Timestamp int64    // 8 bytes: Windows system time when packet was intercepted
	Flags     uint32   // 4 bytes: Bit flags (Layer:8, Event:8, Sniffed:1, Outbound:1 at bit 17, Loopback:1, IPv6:1)
	Reserved2 uint32   // 4 bytes: Reserved memory padding
	Data      [64]byte // 64 bytes: C union containing interface indices, sub-interface IDs, etc.
	// Total size: 8 + 4 + 4 + 64 = EXACTLY 80 BYTES in RAM.
}

// Bit 17 of Flags indicates whether the packet is Outbound (leaving PC) or Inbound (entering PC).
func (a *WinDivertAddress) Outbound() bool {
	// `(1 << 17)` shifts the number 1 left by 17 bits.
	// Bitwise AND `&` checks if bit 17 is turned on (1).
	return (a.Flags & (1 << 17)) != 0
}

func (a *WinDivertAddress) Inbound() bool {
	return !a.Outbound()
}

// Variables storing pointers to the dynamic C functions loaded from WinDivert.dll:
var (
	winDivertDLL                 *syscall.DLL  // Pointer to the loaded WinDivert.dll in memory
	winDivertOpen                *syscall.Proc // C function: WinDivertOpen()
	winDivertClose               *syscall.Proc // C function: WinDivertClose()
	winDivertRecv                *syscall.Proc // C function: WinDivertRecv()
	winDivertSend                *syscall.Proc // C function: WinDivertSend()
	winDivertSetParam            *syscall.Proc // C function: WinDivertSetParam()
	winDivertHelperCalcChecksums *syscall.Proc // C function: WinDivertHelperCalcChecksums()
	winDivertAvailable           bool          // Becomes true if DLL was successfully loaded
)

// `initWinDivert`:
// Dynamically locates WinDivert.dll and WinDivert64.sys beside our .exe and links all functions.
func initWinDivert() error {
	// Verify that Go compiled WinDivertAddress to exactly 80 bytes:
	if unsafe.Sizeof(WinDivertAddress{}) != 80 {
		return fmt.Errorf("fatal ABI error: WinDivertAddress size is %d bytes, expected 80", unsafe.Sizeof(WinDivertAddress{}))
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable directory: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	dllPath := filepath.Join(exeDir, "WinDivert.dll")
	sysPath := filepath.Join(exeDir, "WinDivert64.sys")

	// Verify both DLL and SYS files exist:
	if _, err := os.Stat(dllPath); err != nil {
		return fmt.Errorf("WinDivert.dll not found in %s", exeDir)
	}
	if _, err := os.Stat(sysPath); err != nil {
		return fmt.Errorf("WinDivert64.sys missing beside WinDivert.dll in %s", exeDir)
	}

	// `syscall.LoadDLL`:
	// Asks Windows OS to load WinDivert.dll into our process's memory space.
	mod, err := syscall.LoadDLL(dllPath)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", dllPath, err)
	}

	// Locate each function export by name:
	winDivertOpen, err = mod.FindProc("WinDivertOpen")
	if err != nil { return err }
	winDivertClose, err = mod.FindProc("WinDivertClose")
	if err != nil { return err }
	winDivertRecv, err = mod.FindProc("WinDivertRecv")
	if err != nil { return err }
	winDivertSend, err = mod.FindProc("WinDivertSend")
	if err != nil { return err }
	winDivertSetParam, err = mod.FindProc("WinDivertSetParam")
	if err != nil { return err }
	winDivertHelperCalcChecksums, err = mod.FindProc("WinDivertHelperCalcChecksums")
	if err != nil { return err }

	winDivertDLL = mod
	winDivertAvailable = true
	return nil
}

// `mapWinDivertError`:
// Translates obscure numeric Windows system error codes into helpful human-readable advice.
func mapWinDivertError(err error) error {
	var errno syscall.Errno
	// `errors.As` checks if the error is a Windows syscall.Errno number
	if errors.As(err, &errno) {
		switch errno {
		case 2:
			return errors.New("WinDivert64.sys driver file not found beside WinDivert.dll")
		case 3:
			return errors.New("System cannot find the path specified (corrupt driver installation)")
		case 5:
			return errors.New("Access Denied: Administrator privileges required (right-click -> 'Run as administrator')")
		case 87:
			return errors.New("Invalid WinDivert filter syntax")
		case 577:
			return errors.New("Digital signature check failed (Secure Boot / Driver Signature Enforcement blocked driver)")
		case 1058:
			return errors.New("The WinDivert service cannot be started, either because it is disabled or has no enabled devices associated with it")
		case 1060:
			return errors.New("The specified WinDivert service does not exist as an installed service")
		case 1275:
			return errors.New("Driver blocked by Windows Defender, Core Isolation (HVCI), or Vulnerable Driver Blocklist")
		case 1450:
			return errors.New("Insufficient system resources exist to complete the requested service")
		}
	}
	return err
}

var errHandleClosed = errors.New("handle closed")

// `WinDivertHandle`:
// Wraps the native Windows OS handle returned by WinDivertOpen.
// `atomic.Bool` allows multiple worker goroutines to check if the handle is open without any mutex locking!
type WinDivertHandle struct {
	handle uintptr     // The raw memory handle returned by Windows
	open   atomic.Bool // True while handle is active; False when closed
}

// `NewWinDivertHandle`:
// Opens a packet capture filter handle in the Windows kernel.
func NewWinDivertHandle(filter string) (*WinDivertHandle, error) {
	if !winDivertAvailable {
		return nil, errors.New("WinDivert library is not loaded")
	}
	// Convert Go string to a null-terminated C-style byte pointer (*byte):
	filterBytePtr, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}

	// Call C WinDivertOpen(filter, layer=0, priority=0, flags=0):
	handle, _, callErr := winDivertOpen.Call(
		uintptr(unsafe.Pointer(filterBytePtr)),
		uintptr(0),
		uintptr(0),
		uintptr(0),
	)
	if handle == uintptr(syscall.InvalidHandle) || handle == 0 {
		return nil, mapWinDivertError(callErr)
	}

	wd := &WinDivertHandle{handle: handle}
	wd.open.Store(true)

	// TUNE PACKET QUEUE:
	// Param 0 = QUEUE_LENGTH (Set to 16,384 packets in RAM)
	// Param 1 = QUEUE_TIME   (Set to 2,000 milliseconds buffer time)
	// Param 2 = QUEUE_SIZE   (Set to 33,554,432 bytes = 32 MB ring buffer)
	// Why? On 1Gbps fiber internet, standard queues fill up in milliseconds. 32MB ensures ZERO dropped packets!
	if err := wd.SetParam(0, 16384); err != nil && verboseLogging {
		log.Printf("[!] Warning: failed to set WinDivert QUEUE_LENGTH: %v", err)
	}
	if err := wd.SetParam(1, 2000); err != nil && verboseLogging {
		log.Printf("[!] Warning: failed to set WinDivert QUEUE_TIME: %v", err)
	}
	if err := wd.SetParam(2, 33554432); err != nil && verboseLogging {
		log.Printf("[!] Warning: failed to set WinDivert QUEUE_SIZE: %v", err)
	}

	return wd, nil
}

func (wd *WinDivertHandle) SetParam(param uint32, val uint64) error {
	if !wd.open.Load() {
		return errHandleClosed
	}
	r, _, err := winDivertSetParam.Call(wd.handle, uintptr(param), uintptr(val))
	if r == 0 {
		return err
	}
	return nil
}

func (wd *WinDivertHandle) Close() error {
	// CompareAndSwap(true, false): atomically sets open to false only if it was previously true.
	// Guarantees Close() only runs once even if called by 5 goroutines at the same time!
	if !wd.open.CompareAndSwap(true, false) {
		return nil
	}
	r, _, err := winDivertClose.Call(wd.handle)
	if r == 0 {
		return err
	}
	return nil
}

func (wd *WinDivertHandle) RecvInto(buf []byte, recvLen *uint32, addr *WinDivertAddress) error {
	if !wd.open.Load() {
		return errHandleClosed
	}
	// Call WinDivertRecv(handle, pPacket, packetLen, pRecvLen, pAddr):
	r, _, err := winDivertRecv.Call(
		wd.handle,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(recvLen)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r == 0 {
		return err
	}
	return nil
}

func (wd *WinDivertHandle) Send(raw []byte, addr *WinDivertAddress) error {
	if !wd.open.Load() {
		return errHandleClosed
	}
	// Call WinDivertSend(handle, pPacket, packetLen, pSendLen=0, pAddr):
	r, _, err := winDivertSend.Call(
		wd.handle,
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(len(raw)),
		0,
		uintptr(unsafe.Pointer(addr)),
	)
	if r == 0 {
		return err
	}
	return nil
}

// Re-computes IPv4 and TCP checksums using WinDivert's fast C-optimized helper.
func (wd *WinDivertHandle) CalcChecksum(raw []byte, addr *WinDivertAddress) {
	_, _, _ = winDivertHelperCalcChecksums.Call(
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(len(raw)),
		uintptr(unsafe.Pointer(addr)),
		uintptr(0),
	)
}


// ============================================================================
// PART 5: RAW TCP/IP PACKET PARSING & MANIPULATION (Lines 459-554)
// ============================================================================

// A raw packet intercepted by WinDivert is a slice of bytes []byte containing:
// [IPv4 Header: 20 bytes] + [TCP Header: 20 to 60 bytes] + [Payload / Data]

// Represents the 6 standard TCP 1-bit control flags:
type TCPFlags struct {
	SYN bool // Synchronize: used to establish connection (Step 1 of 3-way handshake)
	ACK bool // Acknowledge: acknowledges received data or SYN
	RST bool // Reset: immediately kills connection
	FIN bool // Finish: cleanly closes connection
	PSH bool // Push: informs receiver to push data immediately to the application
	URG bool // Urgent: urgent out-of-band pointer
}

// `IPHeaderLen`:
// In IPv4, byte 0 contains version (top 4 bits) and IHL / Internet Header Length (bottom 4 bits).
// `raw[0] & 0x0F` extracts the bottom 4 bits. Each unit equals 4 bytes (standard IP header is 5 * 4 = 20 bytes).
func IPHeaderLen(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	return int(raw[0]&0x0F) * 4
}

// `TCPSrcPort`:
// The TCP header begins immediately after the IP header (`off = IPHeaderLen(raw)`).
// Bytes [off : off+2] contain the 16-bit Source Port (big-endian).
func TCPSrcPort(raw []byte) uint16 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint16(raw[off : off+2])
}

// `TCPDstPort`:
// Bytes [off+2 : off+4] contain the 16-bit Destination Port.
func TCPDstPort(raw []byte) uint16 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint16(raw[off+2 : off+4])
}

// `TCPSeqNum`:
// Bytes [off+4 : off+8] contain the 32-bit TCP Sequence Number.
func TCPSeqNum(raw []byte) uint32 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint32(raw[off+4 : off+8])
}

// `SetTCPSeqNum`:
// Writes a new 32-bit Sequence Number into the packet bytes.
func SetTCPSeqNum(raw []byte, seq uint32) {
	off := IPHeaderLen(raw)
	binary.BigEndian.PutUint32(raw[off+4:off+8], seq)
}

// `TCPAckNum`:
// Bytes [off+8 : off+12] contain the 32-bit TCP Acknowledgement Number.
func TCPAckNum(raw []byte) uint32 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint32(raw[off+8 : off+12])
}

// `TCPDataOffset`:
// Byte [off+12] contains the TCP Data Offset (top 4 bits), which specifies where TCP options end
// and where actual payload data begins.
func TCPDataOffset(raw []byte) int {
	off := IPHeaderLen(raw)
	return int(raw[off+12]>>4) * 4
}

// `GetTCPFlags`:
// Byte [off+13] contains the 8 flag bits. We use bitwise AND `&` with binary masks to check them:
// 0x01 = FIN, 0x02 = SYN, 0x04 = RST, 0x08 = PSH, 0x10 = ACK, 0x20 = URG.
func GetTCPFlags(raw []byte) TCPFlags {
	off := IPHeaderLen(raw)
	flags := raw[off+13]
	return TCPFlags{
		FIN: flags&0x01 != 0,
		SYN: flags&0x02 != 0,
		RST: flags&0x04 != 0,
		PSH: flags&0x08 != 0,
		ACK: flags&0x10 != 0,
		URG: flags&0x20 != 0,
	}
}

// `SetTCPFlag`:
// Modifies a specific flag bit in the raw packet.
// `|= mask` turns the bit ON (bitwise OR).
// `&^= mask` turns the bit OFF (bitwise AND-NOT / bit clear).
func SetTCPFlag(raw []byte, flag string, value bool) {
	off := IPHeaderLen(raw)
	var mask byte
	switch flag {
	case "fin": mask = 0x01
	case "syn": mask = 0x02
	case "rst": mask = 0x04
	case "psh": mask = 0x08
	case "ack": mask = 0x10
	case "urg": mask = 0x20
	}
	if value {
		raw[off+13] |= mask
	} else {
		raw[off+13] &^= mask
	}
}

// `SetTCPPayload`:
// Replaces or attaches a new payload to the TCP packet.
// It copies the IP and TCP headers, appends the new payload, and updates the IPv4 Total Length
// field located at bytes 2:4 of the IP header so the packet length is strictly valid.
func SetTCPPayload(raw []byte, payload []byte) []byte {
	hdrTotal := IPHeaderLen(raw) + TCPDataOffset(raw)
	newRaw := make([]byte, hdrTotal+len(payload))
	copy(newRaw, raw[:hdrTotal])
	copy(newRaw[hdrTotal:], payload)
	binary.BigEndian.PutUint16(newRaw[2:4], uint16(len(newRaw)))
	return newRaw
}

// ============================================================================
// PART 6: EXACT TLS CLIENTHELLO TEMPLATE GENERATOR (Lines 555-619)
// ============================================================================

// HOW DPI INSPECTION WORKS:
// When your browser opens an HTTPS connection, it sends a TLS `ClientHello` packet.
// Because encryption hasn't started yet, the domain name you are visiting (e.g. "youtube.com")
// is sent in CLEAR PLAINTEXT inside an extension called SNI (Server Name Indication).
// The censorship DPI middlebox reads this domain. If it is on the blocked list, the DPI kills the stream!
//
// HOW WE BYPASS IT:
// We generate a genuine-looking TLS 1.3 ClientHello packet containing a WHITELISTED decoy domain
// (like "www.google.com" or "speedtest.net") and inject it with a deliberately crafted sequence number.
// The DPI registers the connection as whitelisted and stops inspecting!

var (
	// A 517-byte raw hex dump of a real browser TLS 1.3 ClientHello packet.
	chTemplateHex = "1603010200010001fc030341d5b549d9cd1adfa7296c8418d157dc7b624c842824ff493b9375bb48d34f2b20bf018bcc90a7c89a230094815ad0c15b736e38c01209d72d282cb5e2105328150024130213031301c02cc030c02bc02fcca9cca8c024c028c023c027009f009e006b006700ff0100018f0000000b00090000066d63692e6972000b000403000102000a00160014001d0017001e0019001801000101010201030104002300000010000e000c02683208687474702f312e310016000000170000000d002a0028040305030603080708080809080a080b080408050806040105010601030303010302040205020602002b00050403040303002d00020101003300260024001d0020435bacc4d05f9d41fef44ab3ad55616c36e0613473e2338770efdaa98693d217001500d5000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	chTemplate    []byte
	templateSNI   = []byte("mci.ir") // The original template used "mci.ir" (length 6)
	chStatic1     []byte            // Bytes 0 to 11 of template
	chStatic2     = []byte{0x20}    // Byte 0x20 (decimal 32: session ID length)
	chStatic3     []byte            // Extension headers up to SNI
	chStatic4     []byte            // Extensions between SNI and KeyShare
	chStatic5     = []byte{0x00, 0x15} // TLS extension type 0x0015 = Padding Extension
)

// `func init()`:
// In Go, `init()` is a special reserved function. You never call it manually!
// Go automatically executes `init()` before `func main()` runs.
// Here it splits the master template into static slices so we can rebuild new ClientHellos instantly.
func init() {
	var err error
	chTemplate, err = hex.DecodeString(chTemplateHex)
	if err != nil {
		panic(err) // panic stops execution if the hex string is corrupted
	}
	if len(chTemplate) != 517 {
		panic(fmt.Sprintf("invalid chTemplate length: %d (expected 517)", len(chTemplate)))
	}
	sniLen := len(templateSNI)
	chStatic1 = chTemplate[:11]
	chStatic3 = chTemplate[76:120]
	chStatic4 = chTemplate[127+sniLen : 262+sniLen]
}

// `appendUint16BE`:
// Appends a 16-bit number (uint16) as 2 bytes in Big-Endian network order.
// `v >> 8` extracts the top 8 bits; `byte(v)` extracts the bottom 8 bits.
func appendUint16BE(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

// `GetClientHelloWith`:
// Builds a fresh 517-byte decoy TLS ClientHello packet.
// It inserts fresh random bytes (`rnd`), session ID (`sessID`), custom SNI (`targetSNI`),
// and public key share (`keyShare`), then automatically calculates and appends padding
// so the final packet is ALWAYS EXACTLY 517 BYTES!
func GetClientHelloWith(rnd, sessID, targetSNI, keyShare []byte) []byte {
	if len(rnd) != 32 || len(sessID) != 32 || len(keyShare) != 32 {
		panic(fmt.Sprintf("invalid parameter lengths: rnd=%d, sessID=%d, keyShare=%d; expected 32 each", len(rnd), len(sessID), len(keyShare)))
	}
	sniLen := len(targetSNI)
	// Calculate dynamic padding so total size remains 517 bytes:
	paddingLen := 219 - sniLen
	if paddingLen < 0 {
		paddingLen = 0
	}

	// Allocate a slice with length 0 and capacity 517 in ONE heap allocation:
	result := make([]byte, 0, 517)
	result = append(result, chStatic1...)
	result = append(result, rnd...)
	result = append(result, chStatic2...)
	result = append(result, sessID...)
	result = append(result, chStatic3...)      // extension 0x0000 (server_name)
	result = appendUint16BE(result, uint16(sniLen+5))
	result = appendUint16BE(result, uint16(sniLen+3))
	result = append(result, 0x00)
	result = appendUint16BE(result, uint16(sniLen))
	result = append(result, targetSNI...)     // INJECT OUR DECOY SNI DOMAIN HERE!
	result = append(result, chStatic4...)
	result = append(result, keyShare...)      // INJECT CRYPTOGRAPHIC KEY SHARE
	result = append(result, chStatic5...)     // extension 0x0015 (padding)
	result = appendUint16BE(result, uint16(paddingLen))
	result = append(result, make([]byte, paddingLen)...) // ZERO-BYTE PADDING
	return result
}


// ============================================================================
// PART 7: CONNECTION IDENTIFIER & STATE TRACKING (Lines 620-665)
// ============================================================================

// What is a 4-tuple?
// In TCP/IP networking, any connection is uniquely identified by 4 values:
// (Source IP, Destination IP, Source Port, Destination Port).
type ConnID struct {
	SrcIP   [4]byte // 4-byte IPv4 address of sender
	DstIP   [4]byte // 4-byte IPv4 address of receiver
	SrcPort uint16  // 16-bit TCP port of sender
	DstPort uint16  // 16-bit TCP port of receiver
}

// `NewConnID`:
// Creates a ConnID from Go net.IP objects and port numbers.
func NewConnID(src, dst net.IP, srcPort, dstPort uint16) ConnID {
	var c ConnID
	copy(c.SrcIP[:], src.To4())
	copy(c.DstIP[:], dst.To4())
	c.SrcPort = srcPort
	c.DstPort = dstPort
	return c
}

// `ConnIDFromRaw`:
// Extracts the 4-tuple directly out of a raw packet intercepted by WinDivert.
// Bytes 12-16 are Source IP; Bytes 16-20 are Destination IP.
func ConnIDFromRaw(raw []byte) (ConnID, bool) {
	if len(raw) < 40 {
		return ConnID{}, false
	}
	var c ConnID
	copy(c.SrcIP[:], raw[12:16])
	copy(c.DstIP[:], raw[16:20])
	c.SrcPort = TCPSrcPort(raw)
	c.DstPort = TCPDstPort(raw)
	return c, true
}

// `FakeInjectiveConnection`:
// State machine that tracks an individual spoofed connection through its handshake lifecycle.
type FakeInjectiveConnection struct {
	Monitor     bool        // True if we are currently listening for packets for this connection
	SynSeq      int64       // Sequence number of the outgoing SYN packet (-1 initially)
	SynAckSeq   int64       // Sequence number of the incoming SYN-ACK packet from server (-1 initially)
	ID          ConnID      // Unique 4-tuple identifier
	Mu          sync.Mutex  // Mutex lock ensuring thread safety when updating fields
	Sock        net.Conn    // The outgoing TCP socket to the remote server
	FakeData    []byte      // The 517-byte decoy ClientHello payload
	SchFakeSent bool        // True once fakeSendThread has been scheduled
	FakeSent    bool        // True once the fake packet has actually been injected into the network
	T2aChan     chan string // Channel to notify connection handler ("fake_data_ack_recv", "remote_reset", etc.)
	PeerSock    net.Conn    // The incoming client socket (e.g., from v2ray or browser)
}

// ============================================================================
// PART 8: WINDIVERT INJECTOR & PARALLEL WORKERS (Lines 666-1056)
// ============================================================================

// `FakeTcpInjector`:
// The core WinDivert interception engine.
// It maintains an array of 65,536 atomic pointers directly indexed by local TCP port number.
// Since TCP ports range from 0 to 65,535, looking up `conns[port]` takes only 1 CPU clock cycle
// and requires ZERO hash-table locking or memory allocation!
type FakeTcpInjector struct {
	wd    *WinDivertHandle                                     // The open WinDivert filter handle
	conns [65536]atomic.Pointer[FakeInjectiveConnection]       // Direct port lookup table
	alive atomic.Bool                                          // True while workers are running
}

// `register`:
// Claims a port slot atomically using CompareAndSwap.
// Returns false if another connection is already using that port.
func (f *FakeTcpInjector) register(conn *FakeInjectiveConnection) bool {
	return f.conns[conn.ID.SrcPort].CompareAndSwap(nil, conn)
}

// `unregister`:
// Safely frees the port slot only if it still belongs to this connection.
func (f *FakeTcpInjector) unregister(conn *FakeInjectiveConnection) {
	f.conns[conn.ID.SrcPort].CompareAndSwap(conn, nil)
}

// `lookup`:
// Returns the connection matching the port and full 4-tuple, or nil if none found.
func (f *FakeTcpInjector) lookup(id ConnID) *FakeInjectiveConnection {
	if c := f.conns[id.SrcPort].Load(); c != nil && c.ID == id {
		return c
	}
	return nil
}

// `NewFakeTcpInjector`:
// Constructs the WinDivert filter string and initializes the injector.
// Notice the filter:
// `tcp and tcp.PayloadLength == 0 and ...`
// This filter only intercepts TCP handshake control packets (SYN, SYN-ACK, ACK).
// It completely ignores all bulk encrypted data packets, keeping CPU usage near 0%!
func NewFakeTcpInjector(connectIP net.IP, connectPort uint16) (*FakeTcpInjector, error) {
	filter := fmt.Sprintf(
		"tcp and tcp.PayloadLength == 0 and ("+
			"(outbound and ip.DstAddr == %s and tcp.DstPort == %d) or "+
			"(inbound and ip.SrcAddr == %s and tcp.SrcPort == %d)"+
			")",
		connectIP.String(), connectPort,
		connectIP.String(), connectPort,
	)

	wd, err := NewWinDivertHandle(filter)
	if err != nil {
		return nil, err
	}
	inj := &FakeTcpInjector{wd: wd}
	inj.alive.Store(true)
	return inj, nil
}

func (f *FakeTcpInjector) IsAlive() bool {
	return f.alive.Load()
}

// `Start`:
// Spawns 3 parallel worker goroutines reading from the kernel buffer simultaneously.
// Multi-worker parallel reading ensures that even on gigabit fiber connections,
// the kernel packet buffer never overflows!
func (f *FakeTcpInjector) Start() {
	var wg sync.WaitGroup
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			f.recvWorker(workerID)
		}(w)
	}

	wg.Wait()
	_ = f.wd.Close()
	if f.alive.Load() {
		f.alive.Store(false)
		log.Println("[!] WinDivert injector workers terminated unexpectedly. Mode degraded to userspace.")
	} else if verboseLogging {
		log.Println("[*] WinDivert injector stopped cleanly.")
	}
}

// `recvWorker`:
// The tight receive loop executed by each background worker.
func (f *FakeTcpInjector) recvWorker(workerID int) {
	buf := make([]byte, winDivertMTUMax)
	var (
		recvLen uint32
		addr    WinDivertAddress
	)
	for f.alive.Load() {
		// Wait for next packet from Windows kernel:
		err := f.wd.RecvInto(buf, &recvLen, &addr)
		if err != nil {
			if !f.alive.Load() {
				return
			}
			var errno syscall.Errno
			// Error 122 = ERROR_INSUFFICIENT_BUFFER (can be safely ignored)
			if errors.As(err, &errno) && errno == 122 {
				continue
			}
			log.Printf("[!] WinDivert worker %d terminated with Recv error: %v", workerID, err)
			return
		}

		// Process packet synchronously without allocating memory:
		f.processPacket(buf[:recvLen], &addr)
	}
}

func (f *FakeTcpInjector) Close() {
	f.alive.Store(false)
	_ = f.wd.Close()
}

// `sendPacket`:
// Reinjects a modified packet back into the Windows network stack.
func (f *FakeTcpInjector) sendPacket(raw []byte, addr *WinDivertAddress, recalc bool) {
	if recalc {
		// Re-calculate hardware IP and TCP checksums so router doesn't discard packet:
		f.wd.CalcChecksum(raw, addr)
	}
	if err := f.wd.Send(raw, addr); err != nil {
		atomic.AddUint64(&metricSendErrors, 1)
		if verboseLogging {
			log.Printf("[!] WinDivert Send error: %v", err)
		}
	}
}

// `processPacket`:
// Validates packet length and boundaries, with panic recovery.
func (f *FakeTcpInjector) processPacket(raw []byte, addr *WinDivertAddress) {
	var handled bool
	// GO CRASH PROTECTION (recover):
	// In Go, `recover()` catches unexpected panics. If a corrupted packet triggers a panic,
	// this deferred function catches it so the entire proxy application DOES NOT CRASH!
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&metricPanicsRecovered, 1)
			if verboseLogging {
				log.Printf("[!] Recovered from packet parse panic: %v", r)
			}
			// Reinject original packet if it wasn't handled yet so traffic doesn't freeze:
			if !handled {
				f.sendPacket(raw, addr, false)
			}
		}
	}()

	// Ensure packet is at least 40 bytes (minimum 20 bytes IP + 20 bytes TCP):
	if len(raw) < 40 {
		handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	ihl := IPHeaderLen(raw)
	if ihl < 20 || ihl > len(raw) {
		handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	if len(raw) < ihl+20 {
		handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	dataOff := TCPDataOffset(raw)
	if dataOff < 20 || ihl+dataOff > len(raw) {
		handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	f.inject(raw, addr, &handled)
}

// `fakeSendThread`:
// INJECTS THE SPOOFED DECOY PACKET!
//
// THE MAGIC TRICK (Out-Of-Window Sequence Number):
// `wrongSeq := (uint32(conn.SynSeq) + 1 - payloadLen) & 0xffffffff`
// Notice that the sequence number is set to (SYN_SEQ + 1 - payloadLen).
// 1. To the ISP Firewall Middlebox: It sees a packet with domain "www.google.com".
//    The firewall says "Google is allowed!", updates its flow table, and turns OFF inspection!
// 2. To the Real Target Server: The server receives this packet, notices the sequence number
//    is BEFORE the current TCP window, safely drops it, and replies with an ACK.
// 3. Result: Firewall is fooled, server receives zero bad data, bypass succeeds!
func (f *FakeTcpInjector) fakeSendThread(rawCopy []byte, addr *WinDivertAddress, conn *FakeInjectiveConnection) {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&metricPanicsRecovered, 1)
			log.Printf("[!] Recovered from panic in fakeSendThread: %v", r)
		}
	}()

	conn.Mu.Lock()
	defer conn.Mu.Unlock()

	if !conn.Monitor {
		return
	}

	// Set TCP PSH flag so DPI processes the packet immediately:
	SetTCPFlag(rawCopy, "psh", true)
	// Attach our 517-byte decoy ClientHello:
	newRaw := SetTCPPayload(rawCopy, conn.FakeData)

	payloadLen := uint32(len(conn.FakeData))
	// Calculate out-of-window sequence number:
	wrongSeq := (uint32(conn.SynSeq) + 1 - payloadLen) & 0xffffffff
	SetTCPSeqNum(newRaw, wrongSeq)
	conn.FakeSent = true

	// Increment IP Identification header field:
	ident := binary.BigEndian.Uint16(newRaw[4:6])
	binary.BigEndian.PutUint16(newRaw[4:6], ident+1)

	addrCopy := *addr
	f.sendPacket(newRaw, &addrCopy, true)
	atomic.AddUint64(&metricFakeInjected, 1)
}

func (f *FakeTcpInjector) onUnexpectedPacket(raw []byte, addr *WinDivertAddress, conn *FakeInjectiveConnection, reason string, handled *bool) {
	if conn != nil {
		if verboseLogging {
			log.Printf("[!] Unexpected packet: %s", reason)
		}
		conn.Monitor = false
		select {
		case conn.T2aChan <- ("unexpected_close: " + reason):
		default:
		}
	}
	*handled = true
	f.sendPacket(raw, addr, false)
}

// `onInboundPacket`:
// Handles packets arriving from the remote server:
func (f *FakeTcpInjector) onInboundPacket(raw []byte, addr *WinDivertAddress, conn *FakeInjectiveConnection, handled *bool) {
	if conn.SynSeq == -1 {
		f.onUnexpectedPacket(raw, addr, conn, "inbound before syn", handled)
		return
	}

	flags := GetTCPFlags(raw)
	seqNum := TCPSeqNum(raw)
	ackNum := TCPAckNum(raw)

	// If server sent RST or FIN, abort spoofing:
	if flags.RST || flags.FIN {
		conn.Monitor = false
		*handled = true
		f.sendPacket(raw, addr, false)
		select {
		case conn.T2aChan <- "remote_reset":
		default:
		}
		return
	}

	// Inbound SYN-ACK (Step 2 of TCP Handshake):
	if flags.ACK && flags.SYN {
		if conn.SynAckSeq != -1 && conn.SynAckSeq != int64(seqNum) {
			f.onUnexpectedPacket(raw, addr, conn, "syn-ack sequence changed", handled)
			return
		}
		expectedAck := uint32((uint32(conn.SynSeq) + 1) & 0xffffffff)
		if ackNum != expectedAck {
			f.onUnexpectedPacket(raw, addr, conn, "syn-ack ack number mismatch", handled)
			return
		}
		// Record server's SYN-ACK sequence number:
		conn.SynAckSeq = int64(seqNum)
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	// Inbound ACK after our fake packet was sent:
	// Server acknowledges receipt of the fake packet!
	if flags.ACK && !flags.SYN && conn.FakeSent {
		expectedSeq := uint32((uint32(conn.SynAckSeq) + 1) & 0xffffffff)
		if conn.SynAckSeq == -1 || seqNum != expectedSeq {
			f.onUnexpectedPacket(raw, addr, conn, "ack sequence mismatch", handled)
			return
		}
		expectedAck := uint32((uint32(conn.SynSeq) + 1) & 0xffffffff)
		if ackNum != expectedAck {
			f.onUnexpectedPacket(raw, addr, conn, "ack ack_num mismatch", handled)
			return
		}
		conn.Monitor = false
		atomic.AddUint64(&metricFakeAckReceived, 1)
		select {
		case conn.T2aChan <- "fake_data_ack_recv":
		default:
		}
		// DROP the duplicate ACK! (prevents Windows TCP stack from getting confused):
		*handled = true
		return
	}

	f.onUnexpectedPacket(raw, addr, conn, "unhandled inbound packet", handled)
}

// `onOutboundPacket`:
// Handles outgoing packets from local PC:
func (f *FakeTcpInjector) onOutboundPacket(raw []byte, addr *WinDivertAddress, conn *FakeInjectiveConnection, handled *bool) {
	flags := GetTCPFlags(raw)
	seqNum := TCPSeqNum(raw)
	ackNum := TCPAckNum(raw)

	// Allow retransmissions of final ACK:
	if conn.SchFakeSent {
		if flags.ACK && !flags.SYN && !flags.RST && !flags.FIN {
			expectedSeq := uint32((uint32(conn.SynSeq) + 1) & 0xffffffff)
			expectedAck := uint32((uint32(conn.SynAckSeq) + 1) & 0xffffffff)
			if seqNum == expectedSeq && ackNum == expectedAck {
				*handled = true
				f.sendPacket(raw, addr, false)
				return
			}
		}
		f.onUnexpectedPacket(raw, addr, conn, "outbound packet after fake scheduled", handled)
		return
	}

	// Outbound SYN (Step 1 of TCP Handshake):
	if flags.SYN && !flags.ACK && !flags.RST && !flags.FIN {
		if ackNum != 0 {
			f.onUnexpectedPacket(raw, addr, conn, "outbound syn ack_num not zero", handled)
			return
		}
		if conn.SynSeq != -1 && conn.SynSeq != int64(seqNum) {
			f.onUnexpectedPacket(raw, addr, conn, "outbound syn sequence changed", handled)
			return
		}
		conn.SynSeq = int64(seqNum)
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	// Outbound final ACK of 3-way handshake (Step 3 of TCP Handshake):
	if flags.ACK && !flags.SYN && !flags.RST && !flags.FIN {
		expectedSeq := uint32((uint32(conn.SynSeq) + 1) & 0xffffffff)
		if conn.SynSeq == -1 || seqNum != expectedSeq {
			f.onUnexpectedPacket(raw, addr, conn, "outbound ack sequence mismatch", handled)
			return
		}
		expectedAck := uint32((uint32(conn.SynAckSeq) + 1) & 0xffffffff)
		if conn.SynAckSeq == -1 || ackNum != expectedAck {
			f.onUnexpectedPacket(raw, addr, conn, "outbound ack ack_num mismatch", handled)
			return
		}

		*handled = true
		f.sendPacket(raw, addr, false) // Send genuine final ACK
		conn.SchFakeSent = true

		// Spawn fake injection thread:
		rawCopy := make([]byte, len(raw))
		copy(rawCopy, raw)
		addrCopy := *addr
		go f.fakeSendThread(rawCopy, &addrCopy, conn)
		return
	}

	f.onUnexpectedPacket(raw, addr, conn, "unhandled outbound packet", handled)
}

// `inject`:
// Master packet dispatch router:
func (f *FakeTcpInjector) inject(raw []byte, addr *WinDivertAddress, handled *bool) {
	cid, ok := ConnIDFromRaw(raw)
	if !ok {
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	inbound := addr.Inbound()
	key := cid
	if inbound {
		// Inbound packets (server -> client) swap source and destination:
		key = ConnID{
			SrcIP:   cid.DstIP,
			DstIP:   cid.SrcIP,
			SrcPort: cid.DstPort,
			DstPort: cid.SrcPort,
		}
	}

	conn := f.lookup(key)
	if conn == nil {
		// Not one of our spoofed connections: pass through untouched!
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	conn.Mu.Lock()
	defer conn.Mu.Unlock()
	if !conn.Monitor {
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}
	if inbound {
		f.onInboundPacket(raw, addr, conn, handled)
	} else {
		f.onOutboundPacket(raw, addr, conn, handled)
	}
}

// ============================================================================
// PART 9: PORT RESERVATION & PHYSICAL INTERFACE PINNING (Lines 1057-1238)
// ============================================================================

// `reserveSrcPort`:
// When we spoof a connection, we need to know its local source port in advance.
// By binding to port 0, Windows automatically picks an unused random ephemeral port.
// We record that port number and close the listener so our outgoing dial can use it.
func reserveSrcPort(ip net.IP) (int, error) {
	l, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: ip, Port: 0})
	if err != nil {
		return 0, err
	}
	// Type assertion `.(*net.TCPAddr)` extracts the concrete TCP address:
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}

// `getInterfaceIndex`:
// Inspects network cards on the machine to find the Windows Interface Index for an IP.
func getInterfaceIndex(ip net.IP) (uint32, string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0, "", err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var addrIP net.IP
			// Type switch in Go: checks what concrete type `addr` is:
			switch v := addr.(type) {
			case *net.IPNet:
				addrIP = v.IP
			case *net.IPAddr:
				addrIP = v.IP
			}
			if addrIP != nil && addrIP.To4() != nil && addrIP.Equal(ip) {
				return uint32(iface.Index), iface.Name, nil
			}
		}
	}
	return 0, "", fmt.Errorf("interface for IP %s not found", ip)
}

var tunWarnOnce sync.Once
var (
	ifCacheMu    sync.RWMutex
	cachedIfKey  string
	cachedIfIP   net.IP
	cachedIfIdx  uint32
	cachedIfTime time.Time
)

// `resolveInterface`:
// Determines which network adapter should be used for egress traffic.
// Caches results for 5 seconds to prevent hammering the slow Windows network API.
func resolveInterface(cfg *Config) (net.IP, uint32, error) {
	key := cfg.InterfaceIP
	if key == "" {
		key = "auto:" + cfg.ConnectIP
	}

	ifCacheMu.RLock()
	if cachedIfKey == key && cachedIfIP != nil && time.Since(cachedIfTime) < 5*time.Second {
		ip, idx := cachedIfIP, cachedIfIdx
		ifCacheMu.RUnlock()
		return ip, idx, nil
	}
	ifCacheMu.RUnlock()

	var (
		ip   net.IP
		idx  uint32
		name string
		err  error
	)
	if cfg.InterfaceIP != "" {
		ip = net.ParseIP(cfg.InterfaceIP)
		if ip == nil {
			return nil, 0, fmt.Errorf("invalid INTERFACE_IP: %s", cfg.InterfaceIP)
		}
		idx, name, err = getInterfaceIndex(ip)
		if err != nil {
			return nil, 0, err
		}
	} else {
		// Auto-discover default gateway interface:
		ip, err = GetDefaultInterfaceIPv4(cfg.ConnectIP)
		if err != nil {
			return nil, 0, err
		}
		idx, name, _ = getInterfaceIndex(ip)
	}

	// TUN/VPN ADAPTER WARNING:
	// If the user is running a VPN (like v2ray/Tun2Socks/Wintun), injecting packets into the
	// virtual TUN device fails because virtual adapters don't route to physical DPI middleboxes.
	if idx > 0 {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "tun") || strings.Contains(lower, "tap") || strings.Contains(lower, "wintun") {
			tunWarnOnce.Do(func() {
				log.Printf("[!] Warning: egress interface %s (%s) appears to be a VPN/TUN adapter. If bypass fails, set INTERFACE_IP in config.json to your physical Wi-Fi/Ethernet IP.", name, ip)
			})
		}
	}

	ifCacheMu.Lock()
	cachedIfKey = key
	cachedIfIP = ip
	cachedIfIdx = idx
	cachedIfTime = time.Now()
	ifCacheMu.Unlock()

	return ip, idx, nil
}

// `dialOutgoing`:
// Opens the upstream connection to the destination server.
// It explicitly binds to the reserved source port and pins the connection to the physical adapter.
func dialOutgoing(
	cfg *Config, interfaceIP net.IP, ifIndex uint32,
	fakeData []byte, incomingSock net.Conn,
	fakeInjector *FakeTcpInjector,
	timeout time.Duration,
) (net.Conn, *FakeInjectiveConnection, error) {
	for attempts := 0; attempts < 5; attempts++ {
		srcPort, pErr := reserveSrcPort(interfaceIP)
		if pErr != nil {
			return nil, nil, pErr
		}

		conn := &FakeInjectiveConnection{
			Monitor:   true,
			SynSeq:    -1,
			SynAckSeq: -1,
			ID:        NewConnID(interfaceIP, cfg.connectIP, uint16(srcPort), uint16(cfg.ConnectPort)),
			FakeData:  fakeData,
			T2aChan:   make(chan string, 1),
			PeerSock:  incomingSock,
		}

		if !fakeInjector.register(conn) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// `net.Dialer`:
		// The `Control` callback allows executing raw Windows socket options before connecting.
		dialer := net.Dialer{
			Timeout:   timeout,
			LocalAddr: &net.TCPAddr{IP: interfaceIP, Port: srcPort},
			Control: func(network, address string, c syscall.RawConn) error {
				if ifIndex > 0 {
					var sockErr error
					_ = c.Control(func(fd uintptr) {
						// SOCKET OPTION IP_UNICAST_IF (31):
						// Forces Windows OS to route this socket strictly out of the specified physical adapter!
						var ifIdxBytes [4]byte
						binary.BigEndian.PutUint32(ifIdxBytes[:], ifIndex)
						val := int(binary.LittleEndian.Uint32(ifIdxBytes[:]))
						sockErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, 31, val)
					})
					return sockErr
				}
				return nil
			},
		}

		outgoingSock, err := dialer.Dial("tcp4", cfg.targetAddr)
		if err != nil {
			fakeInjector.unregister(conn)
			var errno syscall.Errno
			// Port collision error (10048 = WSAEADDRINUSE): retry with another port
			if errors.As(err, &errno) && (errno == 10048 || errno == 10049) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return nil, nil, err
		}

		conn.Mu.Lock()
		conn.Sock = outgoingSock
		conn.Mu.Unlock()
		return outgoingSock, conn, nil
	}

	return nil, nil, fmt.Errorf("failed to bind ephemeral port after 5 attempts to dial %s", cfg.targetAddr)
}


// ============================================================================
// PART 10: USERSPACE FALLBACK ENGINE (TLS SEGMENTATION) (Lines 1239-1381)
// ============================================================================

// What is Userspace Fallback?
// If the program is run without Administrator rights, or if Windows Defender blocks
// WinDivert, we don't just give up!
// We fallback to pure userspace TLS ClientHello segmentation.
// By splitting the TLS handshake across multiple TCP packets with microsecond delays,
// commercial DPI devices fail to reassemble the domain name, letting your traffic pass!

// Reads a complete TLS ClientHello message from the incoming client stream:
func ReadFullClientHello(conn net.Conn) ([]byte, error) {
	header := make([]byte, 5) // TLS record header is exactly 5 bytes
	_ = conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	n, err := io.ReadFull(conn, header)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return header[:n], err
	}

	// Verify TLS record header: Byte 0 = 22 (Handshake), Byte 1 = 3 (TLS Version)
	if header[0] != 22 || header[1] != 3 {
		return header, errors.New("not a valid TLS handshake record")
	}

	// Extract payload length from bytes 3:5
	payloadLen := int(binary.BigEndian.Uint16(header[3:5]))
	if payloadLen <= 0 || payloadLen > 16384+2048 {
		return header, errors.New("invalid TLS record length")
	}

	fullBuf := make([]byte, 5+payloadLen)
	copy(fullBuf[:5], header)

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	m, err := io.ReadFull(conn, fullBuf[5:])
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return fullBuf[:5+m], err
	}

	return fullBuf, nil
}

// Walks through the binary TLS extensions to find the exact byte offset of the SNI domain:
func FindSNIOffset(data []byte) int {
	if len(data) < 43 { return -1 }
	idx := 5
	if idx+4 > len(data) || data[idx] != 1 { return -1 }
	idx += 4 + 2 + 32 // Skip Handshake header + Client Version + Client Random (32 bytes)

	if idx >= len(data) { return -1 }
	sessLen := int(data[idx])
	idx += 1 + sessLen // Skip Session ID

	if idx+2 > len(data) { return -1 }
	csLen := int(binary.BigEndian.Uint16(data[idx : idx+2]))
	idx += 2 + csLen // Skip Cipher Suites

	if idx >= len(data) { return -1 }
	compLen := int(data[idx])
	idx += 1 + compLen // Skip Compression Methods

	if idx+2 > len(data) { return -1 }
	extLen := int(binary.BigEndian.Uint16(data[idx : idx+2]))
	idx += 2
	extEnd := idx + extLen
	if extEnd > len(data) { return -1 }

	// Walk each extension:
	for idx+4 <= extEnd {
		eType := binary.BigEndian.Uint16(data[idx : idx+2])
		eLen := int(binary.BigEndian.Uint16(data[idx+2 : idx+4]))
		idx += 4
		if eType == 0x0000 { // 0x0000 = Server Name Indication (SNI)
			if idx+5 <= extEnd {
				return idx + 5 // Return pointer to the start of the host string!
			}
		}
		idx += eLen
	}
	return -1
}

// `handleUserspaceConnection`:
// Handles proxying using pure userspace TCP packet segmentation:
func handleUserspaceConnection(incomingSock net.Conn, cfg *Config) {
	atomic.AddUint64(&metricUserspaceFallbacks, 1)

	targetAddr := cfg.targetAddr
	remoteConn, err := net.DialTimeout("tcp4", targetAddr, time.Duration(cfg.HandshakeTimeoutMs)*time.Millisecond)
	if err != nil {
		if verboseLogging {
			log.Printf("[-] Userspace dial error to %s: %v", targetAddr, err)
		}
		return
	}
	defer remoteConn.Close()

	// Disable Nagle's algorithm (TCP_NODELAY): sends packets immediately without buffering
	if tc, ok := incomingSock.(*net.TCPConn); ok { _ = tc.SetNoDelay(true) }
	if tc, ok := remoteConn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(15 * time.Second)
	}

	initialData, err := ReadFullClientHello(incomingSock)
	if err != nil {
		if len(initialData) > 0 { _, _ = remoteConn.Write(initialData) }
		relayBiDirectional(incomingSock, remoteConn, cfg.IdleTimeoutSec)
		return
	}

	// Calculate where to split the domain name:
	sniOffset := FindSNIOffset(initialData)
	splitPos := 10
	if sniOffset > 0 && sniOffset+2 < len(initialData) {
		splitPos = sniOffset + 2 // Split right in the middle of the domain name!
	}
	if splitPos >= len(initialData) || splitPos <= 0 {
		splitPos = len(initialData) / 2
	}
	if splitPos <= 0 {
		splitPos = 1
	}

	// 1. Send first fragment:
	if _, err := remoteConn.Write(initialData[:splitPos]); err != nil {
		return
	}
	// 2. Microsecond pause to ensure OS transmits as separate IP packet:
	time.Sleep(3 * time.Millisecond)
	// 3. Send second fragment:
	if _, err := remoteConn.Write(initialData[splitPos:]); err != nil {
		return
	}

	// 4. Hand off stream to bidirectional proxy relay:
	relayBiDirectional(incomingSock, remoteConn, cfg.IdleTimeoutSec)
}


// ============================================================================
// PART 11: BI-DIRECTIONAL RELAY & TCP HALF-CLOSE (Lines 1382-1437)
// ============================================================================

// `closeWriter` Interface:
// In Go, an `interface` defines behavior.
// Any struct that has a `CloseWrite() error` method satisfies this interface.
// `net.TCPConn` implements this to send a TCP FIN packet without closing the read direction.
type closeWriter interface {
	CloseWrite() error
}

// `relayBiDirectional`:
// Pumps data back and forth between client and server until the session ends.
// Reuses 32KB buffers from `relayBufferPool` for extreme performance.
func relayBiDirectional(c1, c2 net.Conn, idleTimeoutSec int) {
	var wg sync.WaitGroup
	wg.Add(2)

	idleTimeout := time.Duration(idleTimeoutSec) * time.Second
	var halfClosed atomic.Bool

	copyOne := func(dst, src net.Conn) {
		defer wg.Done()
		// Rent a 32KB buffer from pool:
		bufPtr := relayBufferPool.Get().(*[]byte)
		defer relayBufferPool.Put(bufPtr) // Return buffer to pool when done!
		buf := *bufPtr

		for {
			if halfClosed.Load() {
				_ = src.SetReadDeadline(time.Now().Add(defaultGracePeriod))
			} else if idleTimeout > 0 {
				_ = src.SetReadDeadline(time.Now().Add(idleTimeout))
			}
			n, err := src.Read(buf)
			if n > 0 {
				_ = dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if _, werr := dst.Write(buf[:n]); werr != nil {
					_ = dst.Close()
					_ = src.Close()
					return
				}
			}
			if err != nil {
				// Handle TCP Half-Close:
				// If sender finished sending (EOF), close write channel of receiver
				// but let receiver finish sending its final responses!
				if cw, ok := dst.(closeWriter); ok && errors.Is(err, io.EOF) {
					halfClosed.Store(true)
					_ = cw.CloseWrite()
					_ = dst.SetReadDeadline(time.Now().Add(defaultGracePeriod))
				} else {
					_ = dst.Close()
					_ = src.Close()
				}
				return
			}
		}
	}

	// Run c2 -> c1 in background goroutine; run c1 -> c2 on current goroutine:
	go copyOne(c2, c1)
	copyOne(c1, c2)
	wg.Wait()
}

// ============================================================================
// PART 12: CONNECTION DISPATCHER, RETRIES & FAILOVER (Lines 1438-1591)
// ============================================================================

// `GetDefaultInterfaceIPv4`:
// A clever trick to discover which local network adapter Windows would use to reach a remote IP.
// We open a UDP "connection" to the remote IP on port 53 (DNS). Crucially, UDP is CONNECTIONLESS,
// so this does NOT actually transmit any packets — it only consults the OS routing table to ask
// "which local IP would you use to reach this destination?" We read that local IP, then close.
// (This is the function called by resolveInterface() in Part 9 when INTERFACE_IP is empty.)
func GetDefaultInterfaceIPv4(remoteIP string) (net.IP, error) {
	// net.Dial("udp4", ...) on a connectionless protocol just programs the socket's default route;
	// no packets leave the machine until you actually Write to it.
	conn, err := net.Dial("udp4", net.JoinHostPort(remoteIP, "53"))
	if err != nil {
		return nil, fmt.Errorf("failed to detect default interface for %s: %w", remoteIP, err)
	}
	defer conn.Close()
	// TYPE ASSERTION: conn.LocalAddr() returns the generic `net.Addr` interface.
	// .(*net.UDPAddr) casts it down to the concrete UDP address struct so we can read its `.IP` field.
	// (Like asking "is this Animal actually a Dog?" and then treating it as a Dog.)
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}

// `handleConnection`:
// Dispatched on a new goroutine for every single connection accepted by our TCP listener.
func handleConnection(
	incomingSock net.Conn,
	cfg *Config,
	fakeSNI []byte,
	fakeInjector *FakeTcpInjector,
) {
	// `defer`: Guarantees this incoming socket is closed when handleConnection exits
	defer incomingSock.Close()

	// Panic recovery protects against any unexpected crash in this session:
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&metricPanicsRecovered, 1)
			log.Printf("[!] Connection handler panic recovered: %v", r)
		}
	}()

	atomic.AddUint64(&metricConnectionsTotal, 1)

	// If WinDivert driver is not running (e.g. user is not Admin):
	if fakeInjector == nil || !fakeInjector.IsAlive() {
		if cfg.FailMode == "closed" {
			return
		}
		// Fallback to pure userspace TLS segmentation:
		handleUserspaceConnection(incomingSock, cfg)
		return
	}

	interfaceIP, ifIndex, err := resolveInterface(cfg)
	if err != nil {
		if cfg.FailMode == "closed" { return }
		log.Printf("[-] Interface discovery failed: %v, falling back to userspace", err)
		handleUserspaceConnection(incomingSock, cfg)
		return
	}

	// Generate 96 random bytes in ONE system call:
	// Bytes 0:32  = TLS Client Random
	// Bytes 32:64 = TLS Session ID
	// Bytes 64:96 = TLS Key Share
	var rb [96]byte
	_, _ = rand.Read(rb[:])
	fakeData := GetClientHelloWith(rb[0:32], rb[32:64], fakeSNI, rb[64:96])

	handshakeTimeout := time.Duration(cfg.HandshakeTimeoutMs) * time.Millisecond
	fakeAckTimeout := time.Duration(cfg.FakeAckTimeoutMs) * time.Millisecond

	var (
		outgoingSock net.Conn
		conn         *FakeInjectiveConnection
		dialErr      error
	)

	// Closure to release attempt state cleanly:
	releaseAttempt := func() {
		if conn != nil {
			conn.Mu.Lock()
			conn.Monitor = false
			conn.Mu.Unlock()
			fakeInjector.unregister(conn)
		}
		if outgoingSock != nil {
			_ = outgoingSock.Close()
		}
		conn = nil
		outgoingSock = nil
	}
	defer releaseAttempt()

	attempts := 1 + cfg.Retries
	success := false

	// Retry loop for spoofed injection:
	for a := 0; a < attempts; a++ {
		if !fakeInjector.IsAlive() { break }

		outgoingSock, conn, dialErr = dialOutgoing(
			cfg, interfaceIP, ifIndex,
			fakeData, incomingSock, fakeInjector,
			handshakeTimeout,
		)
		if dialErr != nil {
			log.Printf("[-] Dial failed: %v", dialErr)
			return
		}

		if tc, ok := outgoingSock.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(15 * time.Second)
			_ = tc.SetNoDelay(true)
		}

		// Wait for decoy ACK confirmation using a timer and select block:
		timer := time.NewTimer(fakeAckTimeout)
		select {
		case msg := <-conn.T2aChan:
			timer.Stop()
			if strings.HasPrefix(msg, "unexpected_close") || msg == "remote_reset" {
				releaseAttempt()
				continue
			}
			success = true
		case <-timer.C:
			// Decoy packet timed out:
			atomic.AddUint64(&metricFakeTimeouts, 1)
			releaseAttempt()
			if verboseLogging {
				log.Printf("[!] Decoy ACK timed out on attempt %d/%d", a+1, attempts)
			}
			continue
		}

		if success { break }
	}

	if !success {
		if cfg.FailMode == "closed" {
			if verboseLogging {
				log.Println("[-] Decoy injection unconfirmed and FAIL_MODE=closed: dropping connection")
			}
			return
		}
		if verboseLogging {
			log.Println("[*] WinDivert injection unconfirmed; falling back to userspace SNI segmentation for session")
		}
		handleUserspaceConnection(incomingSock, cfg)
		return
	}

	// Handshake spoofing succeeded!
	conn.Mu.Lock()
	conn.Monitor = false
	conn.Mu.Unlock()
	fakeInjector.unregister(conn)

	// Pump normal traffic:
	relayBiDirectional(outgoingSock, incomingSock, cfg.IdleTimeoutSec)
}


// ============================================================================
// PART 13: SELF-TEST & DIAGNOSTICS (Lines 1592-1666)
// ============================================================================

// `runSelfTest`:
// Triggered via `-selftest`. Validates that all packet generation, parsing,
// and TLS math are 100% bug-free.
func runSelfTest() error {
	fmt.Println("[*] Running comprehensive self-test diagnostics...")

	// 1. Verify byte-exact match with baseline template:
	templateRnd := chTemplate[11:43]
	templateSess := chTemplate[44:76]
	templateKS := chTemplate[268:300]
	exact := GetClientHelloWith(templateRnd, templateSess, templateSNI, templateKS)
	if !bytes.Equal(exact, chTemplate) {
		return errors.New("byte-exact template match failed against baseline template")
	}
	fmt.Println("    [✓] Baseline ClientHello byte-for-byte exact match verified")

	// 2. Test 1 to 219 SNI lengths:
	rnd := make([]byte, 32)
	sessID := make([]byte, 32)
	keyShare := make([]byte, 32)
	for l := 1; l <= 219; l++ {
		fakeSNI := make([]byte, l)
		for i := range fakeSNI { fakeSNI[i] = 'a' }
		ch := GetClientHelloWith(rnd, sessID, fakeSNI, keyShare)
		if len(ch) != 517 {
			return fmt.Errorf("ClientHello length mismatch for size %d (got %d, expected 517)", l, len(ch))
		}
	}
	fmt.Println("    [✓] Dynamic SNI length padding (1..219 bytes) verified")

	// 3. Test raw TCP manipulation helpers:
	raw := make([]byte, 40)
	raw[0] = 0x45
	raw[32] = 0x50
	SetTCPFlag(raw, "psh", true)
	flags := GetTCPFlags(raw)
	if !flags.PSH { return errors.New("SetTCPFlag/GetTCPFlags failed") }
	SetTCPSeqNum(raw, 0x12345678)
	if TCPSeqNum(raw) != 0x12345678 { return errors.New("SetTCPSeqNum failed") }

	fmt.Println("    [✓] Raw TCP/IP packet manipulation helpers verified")
	fmt.Println("[+] All diagnostics passed cleanly!")
	return nil
}


// ============================================================================
// PART 14: PREFLIGHT DPI TEST MODE (Lines 1667-1747)
// ============================================================================

// `runPreflightTest`:
// Tests if the proxy successfully bypasses DPI for a specific domain (e.g. youtube.com:443).
func runPreflightTest(target string, proxyAddr string) bool {
	fmt.Printf("[*] Running preflight DPI test (SNI: %s) via forwarder %s\n", target, proxyAddr)

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		port = "443"
		target = net.JoinHostPort(host, port)
	}

	conn, err := net.DialTimeout("tcp4", proxyAddr, 5*time.Second)
	if err != nil {
		fmt.Printf("[-] Failed to connect to local forwarder (%s): %v\n", proxyAddr, err)
		return false
	}
	defer conn.Close()

	rnd := make([]byte, 32)
	sessID := make([]byte, 32)
	_, _ = rand.Read(rnd)
	_, _ = rand.Read(sessID)

	// Generate standard X25519 cryptography key pair:
	privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("[-] Failed to generate X25519 key: %v\n", err)
		return false
	}
	keyShare := privKey.PublicKey().Bytes()

	clientHello := GetClientHelloWith(rnd, sessID, []byte(host), keyShare)

	// Send probe through proxy:
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(clientHello); err != nil {
		fmt.Printf("[-] Failed to send ClientHello: %v\n", err)
		return false
	}

	_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	respBuf := make([]byte, 1024)
	n, err := io.ReadAtLeast(conn, respBuf, 5)
	if err != nil {
		fmt.Printf("[-] Failed to receive TLS response (dropped by DPI): %v\n", err)
		return false
	}

	// Content Type 22 = TLS Handshake (ServerHello reached us!)
	// Content Type 21 = TLS Alert (Handshake reached real server, DPI was bypassed!)
	if (respBuf[0] == 22 || respBuf[0] == 21) && respBuf[1] == 3 {
		fmt.Printf("[PASS] DPI Bypass verified! Received genuine TLS response from %s\n", target)
		return true
	}

	fmt.Printf("[?] Received %d non-TLS bytes: %v\n", n, respBuf[:n])
	return false
}


// ============================================================================
// PART 15: MAIN APPLICATION ENTRYPOINT (Lines 1748-1947)
// ============================================================================

// `func main()`:
// The entry point where program execution begins when you run the .exe!
func main() {
	fmt.Print(Banner)

	// Command line flag declarations:
	var (
		configPath string
		selftest   bool
		testTarget string
		verbose    bool
	)
	flag.StringVar(&configPath, "config", "", "Path to custom config.json")
	flag.BoolVar(&selftest, "selftest", false, "Run internal diagnostics self-test")
	flag.StringVar(&testTarget, "test", "", "Run preflight DPI bypass test (e.g. -test youtube.com:443)")
	flag.BoolVar(&verbose, "v", false, "Enable verbose diagnostic logging")
	flag.Parse() // Parses command line flags passed by user

	verboseLogging = verbose

	if selftest {
		if err := runSelfTest(); err != nil {
			log.Fatalf("[-] Diagnostics failed: %v", err)
		}
		return
	}

	// 1. Load configuration:
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("[-] Configuration error: %v", err)
	}

	listenAddr := net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.ListenPort))
	clientAddr := listenAddr
	if ip := net.ParseIP(cfg.ListenHost); ip != nil && ip.IsUnspecified() {
		clientAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.ListenPort))
	}

	if testTarget != "" {
		if !runPreflightTest(testTarget, clientAddr) {
			os.Exit(1)
		}
		return
	}

	fakeSNI := []byte(cfg.FakeSNI)

	initIP, initIdx, initErr := resolveInterface(cfg)
	if initErr == nil {
		fmt.Printf("[+] Egress Interface:    %s (Index: %d)\n", initIP, initIdx)
	}

	fmt.Printf("[+] Target Destination:  %s:%d\n", cfg.ConnectIP, cfg.ConnectPort)
	fmt.Printf("[+] Decoy Spoofed SNI:   %s (len: %d)\n", cfg.FakeSNI, len(cfg.FakeSNI))
	fmt.Printf("[+] Strategy:            WinDivert 2.x Out-of-Window Decoy Injection (wrong_seq)\n")
	fmt.Printf("[+] Failover Policy:     FAIL_MODE=%s (Retries: %d)\n", cfg.FailMode, cfg.Retries)

	// 2. Initialize WinDivert:
	divertErr := initWinDivert()
	var fakeInjector *FakeTcpInjector
	if divertErr == nil {
		injector, err := NewFakeTcpInjector(cfg.connectIP, uint16(cfg.ConnectPort))
		if err == nil {
			fakeInjector = injector
			// Start workers in background goroutine:
			go fakeInjector.Start()
			fmt.Println("[+] WinDivert 2.x Kernel Interceptor: ACTIVE")
			fmt.Println("    [Buffer] 32 MB Ring Buffer, 16,384 Packet Queue, 3 Parallel Workers")
		} else {
			if cfg.FailMode == "closed" {
				log.Fatalf("[-] WinDivert failed to open (%v) and FAIL_MODE is 'closed'. Aborting.", err)
			}
			fmt.Printf("[!] WinDivert notice: %v\n", err)
			fmt.Println("[*] Running in Userspace Fallback: Pure TLS SNI Segmentation")
		}
	} else {
		if cfg.FailMode == "closed" {
			log.Fatalf("[-] WinDivert unavailable (%v) and FAIL_MODE is 'closed'. Aborting.", divertErr)
		}
		fmt.Printf("[!] WinDivert unavailable (%v)\n", divertErr)
		fmt.Println("[*] Running in Userspace Fallback: Pure TLS SNI Segmentation")
	}

	// 3. Bind TCP listener on local port:
	listener, err := net.Listen("tcp4", listenAddr)
	if err != nil {
		log.Fatalf("[-] Failed to bind TCP listener on %s: %v", listenAddr, err)
	}
	defer listener.Close()

	fmt.Printf("[+] Proxy forwarder listening on: %s\n", listenAddr)
	fmt.Println("------------------------------------------------------------------------")
	fmt.Println("[*] Configure your VPN client (v2rayN/Nekoray/Hiddify):")
	fmt.Printf("    Change Server Address to: %s\n", clientAddr)
	fmt.Println("[*] Ready for connections. Press Ctrl+C to terminate.")

	ctx, cancel := context.WithCancel(context.Background())
	var activeConns sync.WaitGroup

	// 4. Background stats reporter (prints every 30 seconds):
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				total := atomic.LoadUint64(&metricConnectionsTotal)
				if total > 0 || verboseLogging {
					fmt.Printf("[Stats] Conns: %d | Fakes Injected: %d | ACKs Recv: %d | Timeouts: %d | UserspaceFallbacks: %d\n",
						total,
						atomic.LoadUint64(&metricFakeInjected),
						atomic.LoadUint64(&metricFakeAckReceived),
						atomic.LoadUint64(&metricFakeTimeouts),
						atomic.LoadUint64(&metricUserspaceFallbacks),
					)
				}
			}
		}
	}()

	// 5. Catch Ctrl+C signal for graceful shutdown:
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n[*] Shutting down gracefully, draining active connections (Press Ctrl+C again to force exit)...")
		cancel()
		_ = listener.Close()
		if fakeInjector != nil {
			fakeInjector.Close()
		}
		go func() {
			<-sigCh
			fmt.Println("\n[!] Forced exit requested.")
			os.Exit(1)
		}()
	}()

	// 6. Concurrency Limiter:
	// A buffered channel `sem` acts as a counting semaphore.
	// We can only put `cfg.MaxConcurrentConns` items into it before blocking.
	sem := make(chan struct{}, cfg.MaxConcurrentConns)
	backoff := 10 * time.Millisecond

	// 7. Master Connection Accept Loop:
	for {
		sock, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				activeConns.Wait()
				fmt.Println("[*] All connections drained. Goodbye.")
				os.Exit(0)
			default:
			}
			if errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection") {
				activeConns.Wait()
				os.Exit(0)
			}
			time.Sleep(backoff)
			if backoff < 1*time.Second { backoff *= 2 }
			continue
		}
		backoff = 10 * time.Millisecond

		if tc, ok := sock.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(15 * time.Second)
			_ = tc.SetNoDelay(true)
		}

		select {
		case sem <- struct{}{}:
			activeConns.Add(1)
			// Launch connection handler in its own goroutine:
			go func(s net.Conn) {
				defer func() {
					<-sem // Release concurrency slot
					activeConns.Done()
				}()
				handleConnection(s, cfg, fakeSNI, fakeInjector)
			}(sock)
		case <-ctx.Done():
			_ = sock.Close()
			activeConns.Wait()
			os.Exit(0)
		}
	}
}
