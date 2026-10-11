package core

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientUsesCurrentNetworkResolver(t *testing.T) {
	dns, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dns.Close()
	go func() {
		for {
			buffer := make([]byte, 512)
			n, addr, err := dns.ReadFrom(buffer)
			if err != nil {
				return
			}
			if n < 16 {
				continue
			}
			end := 12
			for end < n && buffer[end] != 0 {
				end += 1 + int(buffer[end])
			}
			end += 5
			if end > n {
				continue
			}
			response := append([]byte(nil), buffer[:end]...)
			binary.BigEndian.PutUint16(response[8:10], 0)
			binary.BigEndian.PutUint16(response[10:12], 0)
			binary.BigEndian.PutUint16(response[2:4], 0x8180)
			if binary.BigEndian.Uint16(buffer[end-4:end-2]) == 1 {
				binary.BigEndian.PutUint16(response[6:8], 1)
				response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 1, 0, 4, 127, 0, 0, 1)
			}
			dns.WriteTo(response, addr) //nolint:errcheck
		}
	}()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", dns.LocalAddr().String())
	}}
	defer func() { net.DefaultResolver = previous }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "resolved") //nolint:errcheck
	}))
	defer server.Close()
	client := NewClient()
	client.Timeout = time.Second
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.Replace(server.URL, "127.0.0.1", "provider.pandaflix.test", 1))
	if err != nil {
		t.Fatalf("current network resolver was bypassed: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "resolved" {
		t.Fatalf("unexpected response %q", body)
	}
}
