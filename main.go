//go:build windows && amd64

// SNI-Spoofing - performance-optimized build by medo
// Production-Grade All-in-One DPI Bypass Application for Windows x64
package main

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	winDivertMTUMax    = 40 + 0xFFFF // 65575 bytes
	defaultMaxConns    = 2000
	defaultGracePeriod = 60 * time.Second
)

// Global metrics
var (
	metricConnectionsTotal   uint64
	metricFakeInjected       uint64
	metricFakeAckReceived    uint64
	metricFakeTimeouts       uint64
	metricPanicsRecovered    uint64
	metricSendErrors         uint64
	metricUserspaceFallbacks uint64
	verboseLogging           bool
)

var relayBufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 32*1024)
		return &b
	},
}

// ============================================================================
// 1. Configuration & Strict Validation
// ============================================================================

type Config struct {
	ListenHost         string `json:"LISTEN_HOST"`
	ListenPort         int    `json:"LISTEN_PORT"`
	ConnectIP          string `json:"CONNECT_IP"`
	ConnectPort        int    `json:"CONNECT_PORT"`
	FakeSNI            string `json:"FAKE_SNI"`
	InterfaceIP        string `json:"INTERFACE_IP,omitempty"`
	FailMode           string `json:"FAIL_MODE,omitempty"` // "closed" or "open"
	HandshakeTimeoutMs int    `json:"HANDSHAKE_TIMEOUT_MS,omitempty"`
	FakeAckTimeoutMs   int    `json:"FAKE_ACK_TIMEOUT_MS,omitempty"`
	IdleTimeoutSec     int    `json:"IDLE_TIMEOUT_SEC,omitempty"` // 0 = disabled (rely on TCP keepalive)
	Retries            int    `json:"RETRIES,omitempty"`
	MaxConcurrentConns int    `json:"MAX_CONCURRENT_CONNS,omitempty"`

	// Derived once at load time (unexported, so never read from config.json). These used to be
	// re-parsed / re-formatted for every single connection.
	connectIP  net.IP
	targetAddr string
}

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

func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

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

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %q: %w", configPath, err)
		}

		// Strip UTF-8 BOM if present (e.g. from Windows Notepad)
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(cfg); err != nil {
			return nil, fmt.Errorf("invalid config.json (check for typos or unknown fields): %w", err)
		}
		if _, err := decoder.Token(); err != io.EOF {
			return nil, errors.New("invalid config.json: unexpected trailing data after JSON object")
		}
		fmt.Printf("[+] Loaded configuration from: %s\n", configPath)
	}

	// Validation
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

	if cfg.ConnectIP == "" {
		return nil, errors.New("CONNECT_IP is required in config.json (e.g. \"172.66.44.137\")")
	}
	parsedConnectIP := net.ParseIP(cfg.ConnectIP)
	if parsedConnectIP == nil || parsedConnectIP.To4() == nil {
		return nil, fmt.Errorf("CONNECT_IP %q must be a valid literal IPv4 address", cfg.ConnectIP)
	}
	if cfg.ConnectPort <= 0 || cfg.ConnectPort > 65535 {
		return nil, fmt.Errorf("CONNECT_PORT %d must be between 1 and 65535", cfg.ConnectPort)
	}
	if cfg.ListenPort <= 0 || cfg.ListenPort > 65535 {
		return nil, fmt.Errorf("LISTEN_PORT %d must be between 1 and 65535", cfg.ListenPort)
	}

	// Validate FAKE_SNI
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

	cfg.connectIP = parsedConnectIP.To4()
	cfg.targetAddr = net.JoinHostPort(cfg.ConnectIP, strconv.Itoa(cfg.ConnectPort))

	return cfg, nil
}

// ============================================================================
// 2. WinDivert 2.x ABI & Dynamic Linker
// ============================================================================

type WinDivertAddress struct {
	Timestamp int64
	Flags     uint32 // Layer:8, Event:8, Sniffed:1, Outbound:1 (bit 17), Loopback:1, Impostor:1, IPv6:1...
	Reserved2 uint32
	Data      [64]byte // Union: 8 + 4 + 4 + 64 = 80 bytes exactly
}

func (a *WinDivertAddress) Outbound() bool {
	return (a.Flags & (1 << 17)) != 0
}

func (a *WinDivertAddress) Inbound() bool {
	return !a.Outbound()
}

var (
	winDivertDLL                 *syscall.DLL
	winDivertOpen                *syscall.Proc
	winDivertClose               *syscall.Proc
	winDivertRecv                *syscall.Proc
	winDivertSend                *syscall.Proc
	winDivertSetParam            *syscall.Proc
	winDivertHelperCalcChecksums *syscall.Proc
	winDivertAvailable           bool
)

func initWinDivert() error {
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

	if _, err := os.Stat(dllPath); err != nil {
		return fmt.Errorf("WinDivert.dll not found in %s", exeDir)
	}
	if _, err := os.Stat(sysPath); err != nil {
		return fmt.Errorf("WinDivert64.sys missing beside WinDivert.dll in %s", exeDir)
	}

	mod, err := syscall.LoadDLL(dllPath)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", dllPath, err)
	}

	winDivertOpen, err = mod.FindProc("WinDivertOpen")
	if err != nil {
		return err
	}
	winDivertClose, err = mod.FindProc("WinDivertClose")
	if err != nil {
		return err
	}
	winDivertRecv, err = mod.FindProc("WinDivertRecv")
	if err != nil {
		return err
	}
	winDivertSend, err = mod.FindProc("WinDivertSend")
	if err != nil {
		return err
	}
	winDivertSetParam, err = mod.FindProc("WinDivertSetParam")
	if err != nil {
		return err
	}
	winDivertHelperCalcChecksums, err = mod.FindProc("WinDivertHelperCalcChecksums")
	if err != nil {
		return err
	}

	winDivertDLL = mod
	winDivertAvailable = true
	return nil
}

func mapWinDivertError(err error) error {
	var errno syscall.Errno
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

// WinDivertHandle is used concurrently by several receive workers and by the sender goroutines.
// The handle value is immutable and the open flag is atomic, so Recv/Send take no lock at all
// (previously every packet took a shared mutex twice).
type WinDivertHandle struct {
	handle uintptr
	open   atomic.Bool
}

func NewWinDivertHandle(filter string) (*WinDivertHandle, error) {
	if !winDivertAvailable {
		return nil, errors.New("WinDivert library is not loaded")
	}
	filterBytePtr, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}
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

	// Maximize buffer parameters to eliminate high-speed fiber packet drops (Issue #37 fix)
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
	if !wd.open.CompareAndSwap(true, false) {
		return nil
	}
	// syscall.Proc.Call always returns a non-nil error value (GetLastError), even on success,
	// so the return value of WinDivertClose (BOOL) must be checked instead.
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
	// pSendLen is optional in WinDivertSend. Passing NULL avoids allocating a result variable
	// for every packet that is reinjected (the hot path).
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

func (wd *WinDivertHandle) CalcChecksum(raw []byte, addr *WinDivertAddress) {
	_, _, _ = winDivertHelperCalcChecksums.Call(
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(len(raw)),
		uintptr(unsafe.Pointer(addr)),
		uintptr(0),
	)
}

// ============================================================================
// 3. Raw TCP/IP Header Accessors
// ============================================================================

type TCPFlags struct {
	SYN bool
	ACK bool
	RST bool
	FIN bool
	PSH bool
	URG bool
}

func IPHeaderLen(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	return int(raw[0]&0x0F) * 4
}

func TCPSrcPort(raw []byte) uint16 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint16(raw[off : off+2])
}

func TCPDstPort(raw []byte) uint16 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint16(raw[off+2 : off+4])
}

func TCPSeqNum(raw []byte) uint32 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint32(raw[off+4 : off+8])
}

func SetTCPSeqNum(raw []byte, seq uint32) {
	off := IPHeaderLen(raw)
	binary.BigEndian.PutUint32(raw[off+4:off+8], seq)
}

func TCPAckNum(raw []byte) uint32 {
	off := IPHeaderLen(raw)
	return binary.BigEndian.Uint32(raw[off+8 : off+12])
}

func TCPDataOffset(raw []byte) int {
	off := IPHeaderLen(raw)
	return int(raw[off+12]>>4) * 4
}

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

func SetTCPFlag(raw []byte, flag string, value bool) {
	off := IPHeaderLen(raw)
	var mask byte
	switch flag {
	case "fin":
		mask = 0x01
	case "syn":
		mask = 0x02
	case "rst":
		mask = 0x04
	case "psh":
		mask = 0x08
	case "ack":
		mask = 0x10
	case "urg":
		mask = 0x20
	}
	if value {
		raw[off+13] |= mask
	} else {
		raw[off+13] &^= mask
	}
}

func SetTCPPayload(raw []byte, payload []byte) []byte {
	hdrTotal := IPHeaderLen(raw) + TCPDataOffset(raw)
	newRaw := make([]byte, hdrTotal+len(payload))
	copy(newRaw, raw[:hdrTotal])
	copy(newRaw[hdrTotal:], payload)
	binary.BigEndian.PutUint16(newRaw[2:4], uint16(len(newRaw)))
	return newRaw
}

// ============================================================================
// 4. Exact TLS ClientHello Template Generator
// ============================================================================

var (
	chTemplateHex = "1603010200010001fc030341d5b549d9cd1adfa7296c8418d157dc7b624c842824ff493b9375bb48d34f2b20bf018bcc90a7c89a230094815ad0c15b736e38c01209d72d282cb5e2105328150024130213031301c02cc030c02bc02fcca9cca8c024c028c023c027009f009e006b006700ff0100018f0000000b00090000066d63692e6972000b000403000102000a00160014001d0017001e0019001801000101010201030104002300000010000e000c02683208687474702f312e310016000000170000000d002a0028040305030603080708080809080a080b080408050806040105010601030303010302040205020602002b00050403040303002d00020101003300260024001d0020435bacc4d05f9d41fef44ab3ad55616c36e0613473e2338770efdaa98693d217001500d5000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	chTemplate    []byte
	templateSNI   = []byte("mci.ir")
	chStatic1     []byte
	chStatic2     = []byte{0x20}
	chStatic3     []byte
	chStatic4     []byte
	chStatic5     = []byte{0x00, 0x15}
)

func init() {
	var err error
	chTemplate, err = hex.DecodeString(chTemplateHex)
	if err != nil {
		panic(err)
	}
	if len(chTemplate) != 517 {
		panic(fmt.Sprintf("invalid chTemplate length: %d (expected 517)", len(chTemplate)))
	}
	sniLen := len(templateSNI)
	chStatic1 = chTemplate[:11]
	chStatic3 = chTemplate[76:120]
	chStatic4 = chTemplate[127+sniLen : 262+sniLen]
}

func appendUint16BE(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

// GetClientHelloWith builds the 517-byte decoy ClientHello in a single allocation. The output is
// byte-for-byte identical to the previous implementation (verified by -selftest).
func GetClientHelloWith(rnd, sessID, targetSNI, keyShare []byte) []byte {
	if len(rnd) != 32 || len(sessID) != 32 || len(keyShare) != 32 {
		panic(fmt.Sprintf("invalid parameter lengths: rnd=%d, sessID=%d, keyShare=%d; expected 32 each", len(rnd), len(sessID), len(keyShare)))
	}
	sniLen := len(targetSNI)
	paddingLen := 219 - sniLen
	if paddingLen < 0 {
		paddingLen = 0
	}

	result := make([]byte, 0, 517)
	result = append(result, chStatic1...)
	result = append(result, rnd...)
	result = append(result, chStatic2...)
	result = append(result, sessID...)
	result = append(result, chStatic3...) // ends with the server_name extension type (0x0000)
	result = appendUint16BE(result, uint16(sniLen+5))
	result = appendUint16BE(result, uint16(sniLen+3))
	result = append(result, 0x00)
	result = appendUint16BE(result, uint16(sniLen))
	result = append(result, targetSNI...)
	result = append(result, chStatic4...)
	result = append(result, keyShare...)
	result = append(result, chStatic5...) // padding extension type (0x0015)
	result = appendUint16BE(result, uint16(paddingLen))
	result = append(result, make([]byte, paddingLen)...)
	return result
}

// ============================================================================
// 5. Connection State & Identifier Tracking
// ============================================================================

type ConnID struct {
	SrcIP   [4]byte
	DstIP   [4]byte
	SrcPort uint16
	DstPort uint16
}

func NewConnID(src, dst net.IP, srcPort, dstPort uint16) ConnID {
	var c ConnID
	copy(c.SrcIP[:], src.To4())
	copy(c.DstIP[:], dst.To4())
	c.SrcPort = srcPort
	c.DstPort = dstPort
	return c
}

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

type FakeInjectiveConnection struct {
	Monitor     bool
	SynSeq      int64
	SynAckSeq   int64
	ID          ConnID
	Mu          sync.Mutex
	Sock        net.Conn
	FakeData    []byte
	SchFakeSent bool
	FakeSent    bool
	T2aChan     chan string
	PeerSock    net.Conn
}

// ============================================================================
// 6. WinDivert Injector with Multi-Worker Parallel Recv
// ============================================================================

// FakeTcpInjector tracks in-flight decoy connections in a table indexed directly by local TCP
// port. Every captured packet needs one lookup, so this replaces the former sync.Map keyed by a
// ConnID struct (which boxed the key into an interface and allocated on every packet) with a
// lock-free, allocation-free array access. The remote IP/port are fixed by the capture filter,
// so the local port plus a full ConnID comparison identifies a connection exactly.
type FakeTcpInjector struct {
	wd    *WinDivertHandle
	conns [65536]atomic.Pointer[FakeInjectiveConnection]
	alive atomic.Bool
}

// register claims the slot of conn's local port. It returns false if another live connection
// already owns that port, telling the caller to pick a different source port.
func (f *FakeTcpInjector) register(conn *FakeInjectiveConnection) bool {
	return f.conns[conn.ID.SrcPort].CompareAndSwap(nil, conn)
}

// unregister releases conn's slot only if conn still owns it, so a late cleanup can never evict
// a newer connection that has since reused the same port.
func (f *FakeTcpInjector) unregister(conn *FakeInjectiveConnection) {
	f.conns[conn.ID.SrcPort].CompareAndSwap(conn, nil)
}

// lookup returns the tracked connection whose local-perspective 4-tuple equals id, or nil.
func (f *FakeTcpInjector) lookup(id ConnID) *FakeInjectiveConnection {
	if c := f.conns[id.SrcPort].Load(); c != nil && c.ID == id {
		return c
	}
	return nil
}

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

func (f *FakeTcpInjector) Start() {
	var wg sync.WaitGroup
	// Run 3 concurrent worker goroutines reading from the same kernel handle
	// Eliminates queue overflow under gigabit traffic (Issue #1 fix)
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

func (f *FakeTcpInjector) recvWorker(workerID int) {
	buf := make([]byte, winDivertMTUMax)
	// Declared once per worker instead of once per packet: syscall.Proc.Call keeps pointers that
	// are converted to uintptr alive, which forces them onto the heap, so per-iteration variables
	// would cost two allocations for every captured packet. Reuse is safe because processPacket
	// is synchronous and never retains raw/addr (deferred work receives its own copies).
	var (
		recvLen uint32
		addr    WinDivertAddress
	)
	for f.alive.Load() {
		err := f.wd.RecvInto(buf, &recvLen, &addr)
		if err != nil {
			if !f.alive.Load() {
				return
			}
			var errno syscall.Errno
			if errors.As(err, &errno) && errno == 122 {
				continue
			}
			log.Printf("[!] WinDivert worker %d terminated with Recv error: %v", workerID, err)
			return
		}

		// Pass buffer slice directly: zero allocations on hot path
		f.processPacket(buf[:recvLen], &addr)
	}
}

func (f *FakeTcpInjector) Close() {
	f.alive.Store(false)
	_ = f.wd.Close()
}

func (f *FakeTcpInjector) sendPacket(raw []byte, addr *WinDivertAddress, recalc bool) {
	if recalc {
		f.wd.CalcChecksum(raw, addr)
	}
	if err := f.wd.Send(raw, addr); err != nil {
		atomic.AddUint64(&metricSendErrors, 1)
		if verboseLogging {
			log.Printf("[!] WinDivert Send error: %v", err)
		}
	}
}

func (f *FakeTcpInjector) processPacket(raw []byte, addr *WinDivertAddress) {
	var handled bool
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&metricPanicsRecovered, 1)
			if verboseLogging {
				log.Printf("[!] Recovered from packet parse panic: %v", r)
			}
			// If panic happened before packet was sent or dropped, reinject it safely! (Issue #2 fix)
			if !handled {
				f.sendPacket(raw, addr, false)
			}
		}
	}()

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

func (f *FakeTcpInjector) fakeSendThread(rawCopy []byte, addr *WinDivertAddress, conn *FakeInjectiveConnection) {
	// This runs in its own goroutine, so the recover() in processPacket does not cover it.
	// Without this, any panic here would terminate the whole process.
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

	SetTCPFlag(rawCopy, "psh", true)
	newRaw := SetTCPPayload(rawCopy, conn.FakeData)

	payloadLen := uint32(len(conn.FakeData))
	wrongSeq := (uint32(conn.SynSeq) + 1 - payloadLen) & 0xffffffff
	SetTCPSeqNum(newRaw, wrongSeq)
	conn.FakeSent = true

	ident := binary.BigEndian.Uint16(newRaw[4:6])
	binary.BigEndian.PutUint16(newRaw[4:6], ident+1)

	addrCopy := *addr
	f.sendPacket(newRaw, &addrCopy, true)
	atomic.AddUint64(&metricFakeInjected, 1)
}

// onUnexpectedPacket: caller MUST hold conn.Mu (Open #1 fix: handler decides socket lifecycle)
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

func (f *FakeTcpInjector) onInboundPacket(raw []byte, addr *WinDivertAddress, conn *FakeInjectiveConnection, handled *bool) {
	if conn.SynSeq == -1 {
		f.onUnexpectedPacket(raw, addr, conn, "inbound before syn", handled)
		return
	}

	flags := GetTCPFlags(raw)
	seqNum := TCPSeqNum(raw)
	ackNum := TCPAckNum(raw)

	// Remote server Reset or FIN
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

	// Inbound SYN-ACK
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
		conn.SynAckSeq = int64(seqNum)
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	// Inbound pure ACK following fake ClientHello injection
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
		// Drop the dup-ACK to keep OS stack clean (Open #15 fix: marked handled before return)
		*handled = true
		return
	}

	f.onUnexpectedPacket(raw, addr, conn, "unhandled inbound packet", handled)
}

func (f *FakeTcpInjector) onOutboundPacket(raw []byte, addr *WinDivertAddress, conn *FakeInjectiveConnection, handled *bool) {
	flags := GetTCPFlags(raw)
	seqNum := TCPSeqNum(raw)
	ackNum := TCPAckNum(raw)

	// In-flight retransmission of final ACK: forward safely without aborting (Issue #5 fix)
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

	// Outbound SYN
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

	// Outbound final ACK of 3-way handshake
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
		f.sendPacket(raw, addr, false)
		conn.SchFakeSent = true

		rawCopy := make([]byte, len(raw))
		copy(rawCopy, raw)
		addrCopy := *addr

		go f.fakeSendThread(rawCopy, &addrCopy, conn)
		return
	}

	f.onUnexpectedPacket(raw, addr, conn, "unhandled outbound packet", handled)
}

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
		// Tracked connections are keyed from the local side, so inbound (server -> local)
		// packets must be looked up with source and destination swapped.
		key = ConnID{
			SrcIP:   cid.DstIP,
			DstIP:   cid.SrcIP,
			SrcPort: cid.DstPort,
			DstPort: cid.SrcPort,
		}
	}

	conn := f.lookup(key)
	if conn == nil {
		// Not one of our decoy connections: reinject untouched (lock-free, allocation-free).
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
// 7. Port Reservation & Egress Interface Pinning (TUN Safe)
// ============================================================================

func reserveSrcPort(ip net.IP) (int, error) {
	l, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: ip, Port: 0})
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}

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

// resolveInterface resolves the outbound interface IP and adapter index with a 5s cache.
// FIX: the cache is now keyed by INTERFACE_IP (or the auto-detect target). Previously an explicit
// INTERFACE_IP bypassed the cache entirely, so every single connection re-enumerated all network
// adapters (a slow Windows API call) and could stall the accept path under load.
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
		ip, err = GetDefaultInterfaceIPv4(cfg.ConnectIP)
		if err != nil {
			return nil, 0, err
		}
		idx, name, _ = getInterfaceIndex(ip)
	}

	if idx > 0 {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "tun") || strings.Contains(lower, "tap") || strings.Contains(lower, "wintun") {
			// Always shown (once): decoys sent on a TUN adapter never reach the real network,
			// which otherwise shows up only as unexplained "decoy ACK timed out" failures.
			tunWarnOnce.Do(func() {
				log.Printf("[!] Warning: egress interface %s (%s) appears to be a VPN/TUN adapter. If the bypass fails, set INTERFACE_IP to your physical adapter's IPv4 address.", name, ip)
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

		// register() claims the port slot atomically. reserveSrcPort() closes its probe listener
		// before Dial() binds, so two goroutines can be handed the same ephemeral port; the loser
		// simply picks another port, and unregister() only ever removes the caller's own entry.
		if !fakeInjector.register(conn) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		dialer := net.Dialer{
			Timeout:   timeout,
			LocalAddr: &net.TCPAddr{IP: interfaceIP, Port: srcPort},
			Control: func(network, address string, c syscall.RawConn) error {
				if ifIndex > 0 {
					var sockErr error
					_ = c.Control(func(fd uintptr) {
						// IP_UNICAST_IF (31) forces packet out the physical interface rather than TUN
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
// 8. Userspace Fallback Engine (Pure TLS SNI Segmentation)
// ============================================================================

func ReadFullClientHello(conn net.Conn) ([]byte, error) {
	header := make([]byte, 5)
	_ = conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	n, err := io.ReadFull(conn, header)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return header[:n], err
	}

	if header[0] != 22 || header[1] != 3 {
		return header, errors.New("not a valid TLS handshake record")
	}

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

func FindSNIOffset(data []byte) int {
	if len(data) < 43 {
		return -1
	}
	idx := 5
	if idx+4 > len(data) || data[idx] != 1 {
		return -1
	}
	idx += 4 + 2 + 32

	if idx >= len(data) {
		return -1
	}
	sessLen := int(data[idx])
	idx += 1 + sessLen

	if idx+2 > len(data) {
		return -1
	}
	csLen := int(binary.BigEndian.Uint16(data[idx : idx+2]))
	idx += 2 + csLen

	if idx >= len(data) {
		return -1
	}
	compLen := int(data[idx])
	idx += 1 + compLen

	if idx+2 > len(data) {
		return -1
	}
	extLen := int(binary.BigEndian.Uint16(data[idx : idx+2]))
	idx += 2
	extEnd := idx + extLen
	if extEnd > len(data) {
		return -1
	}

	for idx+4 <= extEnd {
		eType := binary.BigEndian.Uint16(data[idx : idx+2])
		eLen := int(binary.BigEndian.Uint16(data[idx+2 : idx+4]))
		idx += 4
		if eType == 0x0000 {
			if idx+5 <= extEnd {
				return idx + 5
			}
		}
		idx += eLen
	}
	return -1
}

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

	if tc, ok := incomingSock.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	if tc, ok := remoteConn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		// With IDLE_TIMEOUT_SEC=0 the relay relies on TCP keepalive; without it a silently
		// vanished upstream peer would pin this goroutine and a MAX_CONCURRENT_CONNS slot forever.
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(15 * time.Second)
	}

	initialData, err := ReadFullClientHello(incomingSock)
	if err != nil {
		if len(initialData) > 0 {
			_, _ = remoteConn.Write(initialData)
		}
		relayBiDirectional(incomingSock, remoteConn, cfg.IdleTimeoutSec)
		return
	}

	sniOffset := FindSNIOffset(initialData)
	splitPos := 10
	if sniOffset > 0 && sniOffset+2 < len(initialData) {
		splitPos = sniOffset + 2
	}
	if splitPos >= len(initialData) || splitPos <= 0 {
		splitPos = len(initialData) / 2
	}
	if splitPos <= 0 {
		splitPos = 1
	}

	if _, err := remoteConn.Write(initialData[:splitPos]); err != nil {
		return
	}
	time.Sleep(3 * time.Millisecond)
	if _, err := remoteConn.Write(initialData[splitPos:]); err != nil {
		return
	}

	relayBiDirectional(incomingSock, remoteConn, cfg.IdleTimeoutSec)
}

// ============================================================================
// 9. Bi-Directional Relay with sync.Pool & Half-Close
// ============================================================================

type closeWriter interface {
	CloseWrite() error
}

func relayBiDirectional(c1, c2 net.Conn, idleTimeoutSec int) {
	var wg sync.WaitGroup
	wg.Add(2)

	idleTimeout := time.Duration(idleTimeoutSec) * time.Second
	var halfClosed atomic.Bool

	copyOne := func(dst, src net.Conn) {
		defer wg.Done()
		bufPtr := relayBufferPool.Get().(*[]byte)
		defer relayBufferPool.Put(bufPtr)
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

	// Run one direction on the calling goroutine: saves a goroutine (and its stack) per connection.
	go copyOne(c2, c1)
	copyOne(c1, c2)
	wg.Wait()
}

// ============================================================================
// 10. Connection Dispatcher, Retries & Failover
// ============================================================================

func GetDefaultInterfaceIPv4(remoteIP string) (net.IP, error) {
	conn, err := net.Dial("udp4", net.JoinHostPort(remoteIP, "53"))
	if err != nil {
		return nil, fmt.Errorf("failed to detect default interface for %s: %w", remoteIP, err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}

func handleConnection(
	incomingSock net.Conn,
	cfg *Config,
	fakeSNI []byte,
	fakeInjector *FakeTcpInjector,
) {
	defer incomingSock.Close()
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&metricPanicsRecovered, 1)
			log.Printf("[!] Connection handler panic recovered: %v", r)
		}
	}()

	atomic.AddUint64(&metricConnectionsTotal, 1)

	if fakeInjector == nil || !fakeInjector.IsAlive() {
		if cfg.FailMode == "closed" {
			return
		}
		handleUserspaceConnection(incomingSock, cfg)
		return
	}

	interfaceIP, ifIndex, err := resolveInterface(cfg)
	if err != nil {
		if cfg.FailMode == "closed" {
			return
		}
		log.Printf("[-] Interface discovery failed: %v, falling back to userspace", err)
		handleUserspaceConnection(incomingSock, cfg)
		return
	}

	// One CSPRNG call (one syscall) for all 96 random bytes instead of three.
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

	// releaseAttempt frees the decoy-monitor entry and upstream socket of the most recent dial
	// attempt. It is deferred so it also runs on panics and early returns (previously the cleanup
	// was only registered after a successful attempt, leaking the entry and socket otherwise).
	// Failed attempts call it explicitly; it resets the variables so it never runs twice.
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

	// Injection attempt with retry loop (Issue #7 fix)
	attempts := 1 + cfg.Retries
	success := false

	for a := 0; a < attempts; a++ {
		if !fakeInjector.IsAlive() {
			break
		}

		outgoingSock, conn, dialErr = dialOutgoing(
			cfg, interfaceIP, ifIndex,
			fakeData, incomingSock, fakeInjector,
			handshakeTimeout,
		)
		if dialErr != nil {
			log.Printf("[-] Dial failed: %v", dialErr)
			return // Open #4 fix: dial error is fatal to connection, do not retry or fallback
		}

		if tc, ok := outgoingSock.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(15 * time.Second)
			_ = tc.SetNoDelay(true)
		}

		// NewTimer + Stop instead of time.After so the timer is released as soon as the decoy
		// result arrives rather than lingering until it fires.
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
			atomic.AddUint64(&metricFakeTimeouts, 1)
			releaseAttempt()

			if verboseLogging {
				log.Printf("[!] Decoy ACK timed out on attempt %d/%d", a+1, attempts)
			}
			continue
		}

		if success {
			break
		}
	}

	if !success {
		if cfg.FailMode == "closed" {
			if verboseLogging {
				log.Println("[-] Decoy injection unconfirmed and FAIL_MODE=closed: dropping connection (run with -v for details, or set FAIL_MODE=open)")
			}
			return // Open #2 fix: Fail closed strictly terminates without fallback
		}
		// In open mode: fallback to userspace SNI segmentation
		if verboseLogging {
			log.Println("[*] WinDivert injection unconfirmed; falling back to userspace SNI segmentation for session")
		}
		handleUserspaceConnection(incomingSock, cfg)
		return
	}

	// On success the deferred releaseAttempt closes the upstream socket once the relay ends.
	conn.Mu.Lock()
	conn.Monitor = false
	conn.Mu.Unlock()
	fakeInjector.unregister(conn)

	relayBiDirectional(outgoingSock, incomingSock, cfg.IdleTimeoutSec)
}

// ============================================================================
// 11. Self-Test & Diagnostic Suite
// ============================================================================

func runSelfTest() error {
	fmt.Println("[*] Running comprehensive self-test diagnostics...")

	// 1. Byte-exact template match against baseline
	templateRnd := chTemplate[11:43]
	templateSess := chTemplate[44:76]
	templateKS := chTemplate[268:300]
	exact := GetClientHelloWith(templateRnd, templateSess, templateSNI, templateKS)
	if !bytes.Equal(exact, chTemplate) {
		return errors.New("byte-exact template match failed against baseline template")
	}
	fmt.Println("    [✓] Baseline ClientHello byte-for-byte exact match verified")

	// 2. Test 1..219 SNI lengths
	rnd := make([]byte, 32)
	sessID := make([]byte, 32)
	keyShare := make([]byte, 32)

	for l := 1; l <= 219; l++ {
		fakeSNI := make([]byte, l)
		for i := range fakeSNI {
			fakeSNI[i] = 'a'
		}
		ch := GetClientHelloWith(rnd, sessID, fakeSNI, keyShare)
		if len(ch) != 517 {
			return fmt.Errorf("ClientHello length mismatch for size %d (got %d, expected 517)", l, len(ch))
		}
		recordLen := binary.BigEndian.Uint16(ch[3:5])
		if recordLen != 512 {
			return fmt.Errorf("TLS Record length mismatch for size %d (got %d, expected 512)", l, recordLen)
		}
		offset := FindSNIOffset(ch)
		if offset < 0 || offset+l > len(ch) {
			return fmt.Errorf("FindSNIOffset failed to locate SNI for size %d", l)
		}
		if string(ch[offset:offset+l]) != string(fakeSNI) {
			return fmt.Errorf("Extracted SNI mismatch at offset %d for size %d", offset, l)
		}
	}
	fmt.Println("    [✓] Dynamic SNI length padding (1..219 bytes) verified")

	// 3. Test raw TCP packet helpers
	raw := make([]byte, 40)
	raw[0] = 0x45  // IPv4, IHL=5 (20 bytes of IP header)
	raw[32] = 0x50 // TCP data offset = 5 (20 bytes of TCP header: off = 20, raw[off+12] = raw[32] = 0x50)
	if off := TCPDataOffset(raw); off != 20 {
		return fmt.Errorf("TCPDataOffset failed: got %d, expected 20", off)
	}
	SetTCPFlag(raw, "psh", true)
	flags := GetTCPFlags(raw)
	if !flags.PSH {
		return errors.New("SetTCPFlag/GetTCPFlags failed for PSH flag")
	}
	SetTCPSeqNum(raw, 0x12345678)
	if TCPSeqNum(raw) != 0x12345678 {
		return errors.New("SetTCPSeqNum/TCPSeqNum failed")
	}
	testPayload := []byte("hello")
	newPkt := SetTCPPayload(raw, testPayload)
	if len(newPkt) != 45 {
		return fmt.Errorf("SetTCPPayload length mismatch: got %d, expected 45", len(newPkt))
	}
	if !bytes.Equal(newPkt[40:], testPayload) {
		return errors.New("SetTCPPayload payload mismatch")
	}
	fmt.Println("    [✓] Raw TCP/IP packet manipulation helpers verified")

	fmt.Println("[+] All diagnostics passed cleanly!")
	return nil
}

// ============================================================================
// 12. Preflight DPI Test Mode (-test)
// ============================================================================

// runPreflightTest returns true only when the test passed, so main() can set a meaningful exit code.
func runPreflightTest(target string, proxyAddr string) bool {
	fmt.Printf("[*] Running preflight DPI test (SNI: %s) via forwarder %s\n", target, proxyAddr)

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		port = "443"
		target = net.JoinHostPort(host, port)
	}

	// FIX: GetClientHelloWith() assumes 1..219 printable bytes; a longer host used to produce a
	// silently malformed (wrong-length) ClientHello and a confusing "DPI/server" failure.
	if len(host) < 1 || len(host) > 219 {
		fmt.Printf("[-] Invalid SNI host %q: length (%d) must be between 1 and 219 bytes\n", host, len(host))
		return false
	}
	for i := 0; i < len(host); i++ {
		if c := host[i]; c <= 32 || c >= 127 {
			fmt.Printf("[-] Invalid SNI host %q: contains non-printable/whitespace byte at position %d\n", host, i)
			return false
		}
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

	privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("[-] Failed to generate X25519 key: %v\n", err)
		return false
	}
	keyShare := privKey.PublicKey().Bytes()

	clientHello := GetClientHelloWith(rnd, sessID, []byte(host), keyShare)

	// The forwarder always connects to CONNECT_IP:CONNECT_PORT from config.json; the target given to
	// -test only decides which SNI is placed in the ClientHello.
	fmt.Printf("[*] Sending TLS ClientHello with SNI (%s) through the forwarder to the configured CONNECT_IP...\n", host)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(clientHello); err != nil {
		fmt.Printf("[-] Failed to send ClientHello: %v\n", err)
		return false
	}

	_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	respBuf := make([]byte, 1024)
	// FIX: a bare Read() can legally return fewer than 5 bytes of a TLS record header, which was
	// mis-reported as an unknown response. Require at least the 5-byte record header.
	n, err := io.ReadAtLeast(conn, respBuf, 5)
	if err != nil {
		fmt.Printf("[-] Failed to receive TLS response (server timed out or connection dropped by DPI): %v\n", err)
		return false
	}

	// Content Type 22 = Handshake (ServerHello), Content Type 21 = TLS Alert (DPI passed, reached server)
	if respBuf[0] == 22 && respBuf[1] == 3 {
		fmt.Printf("[PASS] DPI Bypass verified! Received TLS ServerHello (%d bytes) from %s\n", n, target)
		return true
	}
	if respBuf[0] == 21 && respBuf[1] == 3 {
		fmt.Printf("[PASS] DPI Bypass verified! Received TLS Alert from upstream server (%d bytes, reached destination)\n", n)
		return true
	}
	fmt.Printf("[?] Received %d bytes that are not a TLS record: %v\n", n, respBuf[:n])
	return false
}

// ============================================================================
// 13. Main Application Entrypoint
// ============================================================================

func main() {
	fmt.Print(Banner)

	var (
		configPath string
		selftest   bool
		testTarget string
		verbose    bool
	)

	flag.StringVar(&configPath, "config", "", "Path to custom config.json")
	flag.BoolVar(&selftest, "selftest", false, "Run internal packet builder & parser self-tests")
	flag.StringVar(&testTarget, "test", "", "Run preflight DPI bypass test (e.g. -test youtube.com:443)")
	flag.BoolVar(&verbose, "v", false, "Enable verbose diagnostic logging")
	flag.Parse()

	verboseLogging = verbose

	if selftest {
		if err := runSelfTest(); err != nil {
			log.Fatalf("[-] Diagnostics failed: %v", err)
		}
		return
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("[-] Configuration error: %v", err)
	}

	listenAddr := net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.ListenPort))

	// Address that local clients (and -test) should use: a wildcard bind such as 0.0.0.0
	// is not a connectable destination on Windows.
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

	// Initialize WinDivert 2.x
	divertErr := initWinDivert()
	var fakeInjector *FakeTcpInjector
	if divertErr == nil {
		injector, err := NewFakeTcpInjector(cfg.connectIP, uint16(cfg.ConnectPort))
		if err == nil {
			fakeInjector = injector
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

	// Background stats monitor
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
					fmt.Printf("[Stats] Conns: %d | Fakes Injected: %d | ACKs Recv: %d | Timeouts: %d | UserspaceFallbacks: %d | SendErr: %d | PanicsRecovered: %d\n",
						total,
						atomic.LoadUint64(&metricFakeInjected),
						atomic.LoadUint64(&metricFakeAckReceived),
						atomic.LoadUint64(&metricFakeTimeouts),
						atomic.LoadUint64(&metricUserspaceFallbacks),
						atomic.LoadUint64(&metricSendErrors),
						atomic.LoadUint64(&metricPanicsRecovered),
					)
				}
			}
		}
	}()

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
			select {
			case <-sigCh: // second Ctrl+C
				fmt.Println("\n[!] Force exiting immediately.")
				os.Exit(0)
			case <-time.After(10 * time.Second):
				fmt.Println("\n[!] Drain timeout (10s) expired. Exiting.")
				os.Exit(0)
			}
		}()
	}()

	sem := make(chan struct{}, cfg.MaxConcurrentConns)
	backoff := 10 * time.Millisecond

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
			if backoff < 1*time.Second {
				backoff *= 2
			}
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
			go func(s net.Conn) {
				defer func() {
					<-sem
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
