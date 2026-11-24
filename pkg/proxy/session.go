package proxy

import (
	"net"
	"sync"
	"time"

	"go-sni-spoof/pkg/stats"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 32*1024)
		return &b
	},
}

// closeWriter is satisfied by *net.TCPConn and *connWithReader.
type closeWriter interface {
	CloseWrite() error
}

// ForwardSession handles high-performance bi-directional traffic relay
// between the local client and the upstream remote destination.
func ForwardSession(clientConn, remoteConn net.Conn, m *stats.Metrics) {
	if m == nil {
		m = stats.DefaultMetrics
	}
	m.IncConnections()
	defer m.DecConnections()
	defer clientConn.Close()
	defer remoteConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Remote
	go func() {
		defer wg.Done()
		bufPtr := bufferPool.Get().(*[]byte)
		defer bufferPool.Put(bufPtr)

		for {
			_ = clientConn.SetReadDeadline(time.Now().Add(120 * time.Second))
			n, err := clientConn.Read(*bufPtr)
			if n > 0 {
				_ = remoteConn.SetWriteDeadline(time.Now().Add(30 * time.Second))
				wn, werr := remoteConn.Write((*bufPtr)[:n])
				if wn > 0 {
					m.AddUpload(wn)
				}
				if werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}

		// Half-close remote write if supported
		if cw, ok := remoteConn.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
	}()

	// Remote -> Client
	go func() {
		defer wg.Done()
		bufPtr := bufferPool.Get().(*[]byte)
		defer bufferPool.Put(bufPtr)

		for {
			_ = remoteConn.SetReadDeadline(time.Now().Add(120 * time.Second))
			n, err := remoteConn.Read(*bufPtr)
			if n > 0 {
				_ = clientConn.SetWriteDeadline(time.Now().Add(30 * time.Second))
				wn, werr := clientConn.Write((*bufPtr)[:n])
				if wn > 0 {
					m.AddDownload(wn)
				}
				if werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}

		// Half-close client write if supported
		if cw, ok := clientConn.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
	}()

	wg.Wait()
}
