//go:build windows

package spoof

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"go-sni-spoof/pkg/stats"
	"go-sni-spoof/pkg/tlshelper"
)

const (
	winDivertMTUMax = 40 + 0xFFFF // 65575 bytes
)

// WinDivertAddress matches WinDivert 2.x ABI
type WinDivertAddress struct {
	Timestamp int64
	Flags     uint32
	Reserved2 uint32
	Data      [64]byte
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
	winDivertInitOnce            sync.Once
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
		case 5:
			return errors.New("Access Denied: Administrator privileges required")
		case 87:
			return errors.New("Invalid WinDivert filter syntax")
		case 577:
			return errors.New("Digital signature check failed (Secure Boot blocked driver)")
		case 1275:
			return errors.New("Driver blocked by Windows Defender or Core Isolation")
		}
	}
	return err
}

// WinDivertHandle wraps a WinDivert handle
type WinDivertHandle struct {
	handle uintptr
	open   atomic.Bool
}

func NewWinDivertHandle(filter string) (*WinDivertHandle, error) {
	winDivertInitOnce.Do(func() {
		if err := initWinDivert(); err != nil {
			log.Printf("[!] WinDivert initialization failed: %v", err)
		}
	})

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

	// Maximize buffer parameters
	_ = wd.SetParam(0, 16384)
	_ = wd.SetParam(1, 2000)
	_ = wd.SetParam(2, 33554432)

	return wd, nil
}

func (wd *WinDivertHandle) SetParam(param uint32, val uint64) error {
	if !wd.open.Load() {
		return errors.New("handle closed")
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
	r, _, err := winDivertClose.Call(wd.handle)
	if r == 0 {
		return err
	}
	return nil
}

func (wd *WinDivertHandle) RecvInto(buf []byte, recvLen *uint32, addr *WinDivertAddress) error {
	if !wd.open.Load() {
		return errors.New("handle closed")
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
		return errors.New("handle closed")
	}
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

// ConnID identifies a TCP connection by 4-tuple
type ConnID struct {
	SrcIP   [4]byte
	DstIP   [4]byte
	SrcPort uint16
	DstPort uint16
}

// FakeConnection tracks a connection for decoy injection
type FakeConnection struct {
	Monitor     bool
	SynSeq      int64
	SynAckSeq   int64
	ID          ConnID
	Mu          sync.Mutex
	FakeData    []byte
	SchFakeSent bool
	FakeSent    bool
	T2aChan     chan string
	RealCHReady chan []byte
}

// WinDivertInjector manages packet interception and fake ClientHello injection
type WinDivertInjector struct {
	wd      *WinDivertHandle
	conns   [65536]atomic.Pointer[FakeConnection]
	alive   atomic.Bool
	metrics *stats.Metrics
	verbose bool
}

func NewWinDivertInjector(connectIP net.IP, connectPort uint16, m *stats.Metrics, verbose bool) (*WinDivertInjector, error) {
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

	inj := &WinDivertInjector{
		wd:      wd,
		metrics: m,
		verbose: verbose,
	}
	inj.alive.Store(true)
	return inj, nil
}

func (f *WinDivertInjector) register(conn *FakeConnection) bool {
	return f.conns[conn.ID.SrcPort].CompareAndSwap(nil, conn)
}

func (f *WinDivertInjector) unregister(conn *FakeConnection) {
	f.conns[conn.ID.SrcPort].CompareAndSwap(conn, nil)
}

func (f *WinDivertInjector) lookup(id ConnID) *FakeConnection {
	if c := f.conns[id.SrcPort].Load(); c != nil && c.ID == id {
		return c
	}
	return nil
}

func (f *WinDivertInjector) Start() {
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
}

func (f *WinDivertInjector) Close() {
	f.alive.Store(false)
	_ = f.wd.Close()
}

func (f *WinDivertInjector) recvWorker(workerID int) {
	buf := make([]byte, winDivertMTUMax)
	var recvLen uint32
	var addr WinDivertAddress

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
			log.Printf("[!] WinDivert worker %d terminated: %v", workerID, err)
			return
		}

		f.processPacket(buf[:recvLen], &addr)
	}
}

func (f *WinDivertInjector) processPacket(raw []byte, addr *WinDivertAddress) {
	var handled bool
	defer func() {
		if r := recover(); r != nil {
			if f.verbose {
				log.Printf("[!] Recovered from packet parse panic: %v", r)
			}
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
	if ihl < 20 || ihl > len(raw) || len(raw) < ihl+20 {
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

func (f *WinDivertInjector) inject(raw []byte, addr *WinDivertAddress, handled *bool) {
	id := ConnIDFromRaw(raw)
	conn := f.lookup(id)

	if addr.Inbound() {
		if conn != nil {
			conn.Mu.Lock()
			f.onInboundPacket(raw, addr, conn, handled)
			conn.Mu.Unlock()
		} else {
			*handled = true
			f.sendPacket(raw, addr, false)
		}
		return
	}

	// Outbound
	if conn != nil {
		conn.Mu.Lock()
		f.onOutboundPacket(raw, addr, conn, handled)
		conn.Mu.Unlock()
	} else {
		*handled = true
		f.sendPacket(raw, addr, false)
	}
}

func (f *WinDivertInjector) onInboundPacket(raw []byte, addr *WinDivertAddress, conn *FakeConnection, handled *bool) {
	if conn.SynSeq == -1 {
		conn.Monitor = false
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	flags := GetTCPFlags(raw)
	seqNum := TCPSeqNum(raw)
	ackNum := TCPAckNum(raw)

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
			conn.Monitor = false
			*handled = true
			f.sendPacket(raw, addr, false)
			return
		}
		expectedAck := uint32((uint32(conn.SynSeq) + 1) & 0xffffffff)
		if ackNum != expectedAck {
			conn.Monitor = false
			*handled = true
			f.sendPacket(raw, addr, false)
			return
		}
		conn.SynAckSeq = int64(seqNum)
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	// Inbound ACK after fake injection
	if flags.ACK && !flags.SYN && conn.FakeSent {
		expectedSeq := uint32((uint32(conn.SynAckSeq) + 1) & 0xffffffff)
		expectedAck := uint32((uint32(conn.SynSeq) + 1) & 0xffffffff)
		if conn.SynAckSeq != -1 && seqNum == expectedSeq && ackNum == expectedAck {
			conn.Monitor = false
			f.metrics.IncInjected()
			select {
			case conn.T2aChan <- "fake_data_ack_recv":
			default:
			}
			*handled = true
			return
		}
	}

	*handled = true
	f.sendPacket(raw, addr, false)
}

func (f *WinDivertInjector) onOutboundPacket(raw []byte, addr *WinDivertAddress, conn *FakeConnection, handled *bool) {
	flags := GetTCPFlags(raw)
	seqNum := TCPSeqNum(raw)
	ackNum := TCPAckNum(raw)

	// In-flight retransmission after fake scheduled
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
		conn.Monitor = false
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	// Outbound SYN
	if flags.SYN && !flags.ACK && !flags.RST && !flags.FIN {
		if ackNum != 0 {
			conn.Monitor = false
			*handled = true
			f.sendPacket(raw, addr, false)
			return
		}
		if conn.SynSeq != -1 && conn.SynSeq != int64(seqNum) {
			conn.Monitor = false
			*handled = true
			f.sendPacket(raw, addr, false)
			return
		}
		conn.SynSeq = int64(seqNum)
		*handled = true
		f.sendPacket(raw, addr, false)
		return
	}

	// Outbound final ACK
	if flags.ACK && !flags.SYN && !flags.RST && !flags.FIN {
		expectedSeq := uint32((uint32(conn.SynSeq) + 1) & 0xffffffff)
		expectedAck := uint32((uint32(conn.SynAckSeq) + 1) & 0xffffffff)
		if conn.SynSeq != -1 && conn.SynAckSeq != -1 && seqNum == expectedSeq && ackNum == expectedAck {
			conn.SchFakeSent = true
			*handled = true
			f.sendPacket(raw, addr, false)

			// Schedule fake injection
			go f.injectFake(raw, addr, conn)
			return
		}
	}

	conn.Monitor = false
	*handled = true
	f.sendPacket(raw, addr, false)
}

func (f *WinDivertInjector) injectFake(rawACK []byte, addr *WinDivertAddress, conn *FakeConnection) {
	defer func() {
		if r := recover(); r != nil {
			if f.verbose {
				log.Printf("[!] Recovered from panic in injectFake: %v", r)
			}
		}
	}()

	// Wait for real ClientHello from application
	var _ []byte
	select {
	case _ = <-conn.RealCHReady:
		// Got it, but we don't need to inspect it - we just inject the fake
	case <-time.After(3 * time.Second):
		if f.verbose {
			log.Printf("[!] Timeout waiting for real ClientHello")
		}
		select {
		case conn.T2aChan <- "timeout_real_ch":
		default:
		}
		return
	}

	conn.Mu.Lock()
	if !conn.Monitor {
		conn.Mu.Unlock()
		return
	}

	// Build fake packet
	rawCopy := make([]byte, len(rawACK))
	copy(rawCopy, rawACK)

	SetTCPFlag(rawCopy, "psh", true)
	newRaw := SetTCPPayload(rawCopy, conn.FakeData)

	payloadLen := uint32(len(conn.FakeData))
	wrongSeq := (uint32(conn.SynSeq) + 1 - payloadLen) & 0xffffffff
	SetTCPSeqNum(newRaw, wrongSeq)
	conn.FakeSent = true

	ident := binary.BigEndian.Uint16(newRaw[4:6])
	binary.BigEndian.PutUint16(newRaw[4:6], ident+1)

	conn.Mu.Unlock()

	// Send fake packet
	addrCopy := *addr
	f.wd.CalcChecksum(newRaw, &addrCopy)
	f.wd.Send(newRaw, &addrCopy)

	if f.verbose {
		log.Printf("[+] Fake ClientHello injected with wrong sequence")
	}
}

func (f *WinDivertInjector) sendPacket(raw []byte, addr *WinDivertAddress, recalc bool) {
	if recalc {
		f.wd.CalcChecksum(raw, addr)
	}
	_ = f.wd.Send(raw, addr)
}

// Helper functions for TCP packet manipulation
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

type TCPFlags struct {
	SYN bool
	ACK bool
	RST bool
	FIN bool
	PSH bool
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
	}
}

func SetTCPFlag(raw []byte, flag string, value bool) {
	off := IPHeaderLen(raw)
	var mask byte
	switch flag {
	case "psh":
		mask = 0x08
	case "ack":
		mask = 0x10
	case "syn":
		mask = 0x02
	case "fin":
		mask = 0x01
	case "rst":
		mask = 0x04
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

func ConnIDFromRaw(raw []byte) ConnID {
	var c ConnID
	copy(c.SrcIP[:], raw[12:16])
	copy(c.DstIP[:], raw[16:20])
	c.SrcPort = TCPSrcPort(raw)
	c.DstPort = TCPDstPort(raw)
	return c
}

// Generate decoy ClientHello
func GenerateFakeClientHello(fakeSNI string) []byte {
	return tlshelper.GenerateClientHello(fakeSNI)
}
