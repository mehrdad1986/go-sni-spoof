package proxy

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestIsHTTPConnectPrefix(t *testing.T) {
	valid := []byte("CONNECT example.com:443 HTTP/1.1\r\n")
	if !IsHTTPConnectPrefix(valid) {
		t.Fatal("expected IsHTTPConnectPrefix=true")
	}

	invalid := []byte("GET /index.html HTTP/1.1\r\n")
	if IsHTTPConnectPrefix(invalid) {
		t.Fatal("expected IsHTTPConnectPrefix=false for GET")
	}
}

func TestSOCKS5Handshake(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	errChan := make(chan error, 1)
	targetChan := make(chan string, 1)

	go func() {
		target, err := HandleSOCKS5(server, nil)
		if err != nil {
			errChan <- err
			return
		}
		targetChan <- target
	}()

	// Client sends: VER=5, NMETHODS=1, METHOD=0
	_, _ = client.Write([]byte{0x05, 0x01, 0x00})

	// Client reads server method selection (2 bytes: 0x05, 0x00)
	resp := make([]byte, 2)
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Read(resp); err != nil {
		t.Fatalf("client failed to read method response: %v", err)
	}
	if !bytes.Equal(resp, []byte{0x05, 0x00}) {
		t.Fatalf("unexpected method response: %v", resp)
	}

	// Client sends CONNECT to 1.2.3.4:443 (IPv4: 0x01)
	// VER=5, CMD=1, RSV=0, ATYP=1, IP=1.2.3.4, PORT=443 (0x01, 0xbb)
	req := []byte{0x05, 0x01, 0x00, 0x01, 1, 2, 3, 4, 0x01, 0xbb}
	_, _ = client.Write(req)

	// Client reads reply (10 bytes)
	reply := make([]byte, 10)
	if _, err := client.Read(reply); err != nil {
		t.Fatalf("client failed to read connect reply: %v", err)
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		t.Fatalf("unexpected connect reply: %v", reply)
	}

	select {
	case err := <-errChan:
		t.Fatalf("HandleSOCKS5 failed: %v", err)
	case target := <-targetChan:
		if target != "1.2.3.4:443" {
			t.Fatalf("expected target 1.2.3.4:443, got %s", target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for target")
	}
}

