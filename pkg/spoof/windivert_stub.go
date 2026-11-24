//go:build !windows

package spoof

import (
	"errors"
	"net"
	"sync"

	"go-sni-spoof/pkg/stats"
)

// WinDivertInjector stub for non-Windows platforms
type WinDivertInjector struct{}

// ConnID stub
type ConnID struct {
	SrcIP   [4]byte
	DstIP   [4]byte
	SrcPort uint16
	DstPort uint16
}

// FakeConnection stub
type FakeConnection struct {
	Monitor     bool
	SynSeq      int64
	SynAckSeq   int64
	ID          ConnID
	FakeData    []byte
	SchFakeSent bool
	FakeSent    bool
	Mu          sync.Mutex
	T2aChan     chan string
	RealCHReady chan []byte
}

func NewWinDivertInjector(connectIP net.IP, connectPort uint16, m *stats.Metrics, verbose bool) (*WinDivertInjector, error) {
	return nil, errors.New("WinDivert is only available on Windows")
}

func (f *WinDivertInjector) Start()             {}
func (f *WinDivertInjector) Close()             {}
func (f *WinDivertInjector) register(conn *FakeConnection) bool { return false }
func (f *WinDivertInjector) unregister(conn *FakeConnection)    {}
func (f *WinDivertInjector) lookup(id ConnID) *FakeConnection   { return nil }

func GenerateFakeClientHello(fakeSNI string) []byte {
	return nil
}