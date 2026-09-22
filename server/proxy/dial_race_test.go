package proxy

import (
	"net"
	"sync"
	"testing"
	"time"

	"example.com/svcmgr/lib/file"
)

// The dial path used to read node.Username / node.Password directly, while
// UpdateConfig writes them under the node's write lock. That is a data race:
// an operator saving the edit form while a connection is being dialled through
// the node. Run with -race to see the original code fail.
func TestDialPathReadsCredentialsUnderLock(t *testing.T) {
	// A listener that accepts and immediately closes: it only exists so the
	// dial reaches the proxy and reads the credentials on the way.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	for _, r := range portStr {
		port = port*10 + int(r-'0')
	}

	node := file.NewProxyNode()
	node.Id = 8801
	node.Host = host
	node.Port = port
	node.Http = true
	node.Socks5 = true
	node.Username = "user"
	node.Password = "pass"
	node.Enabled = true

	var wg sync.WaitGroup

	// Writer: exactly what saving the edit form does.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			node.UpdateConfig(file.ProxyConfig{
				Name: "n", Host: host, Port: port,
				Username: "user", Password: "pass",
				Http: true, Socks5: true, Enabled: true,
			})
		}
	}()

	// Readers: the dial path plus the log helpers that format the node.
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(deadline) {
			c, err := dialViaProxyNode(node, protoHttp, "127.0.0.1:1", 50*time.Millisecond)
			if err == nil && c != nil {
				_ = c.Close()
			}
			c2, err2 := dialViaProxyNode(node, protoSocks5, "127.0.0.1:1", 50*time.Millisecond)
			if err2 == nil && c2 != nil {
				_ = c2.Close()
			}
			// These are the values the log lines and the tunnel dialer read.
			_ = node.Addr()
			_ = node.String()
			_ = node.SchemeName()
			_ = node.HostValue()
			_, _ = node.Credentials()
		}
	}()

	wg.Wait()
}
