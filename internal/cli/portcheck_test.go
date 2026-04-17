package cli

import (
	"fmt"
	"net"
	"testing"
)

func TestPortAvailable_FreePort(t *testing.T) {
	// Ask the kernel for a free port, close it, then call
	// PortAvailable — there's a small race window where another
	// process could nab the port, but in practice CI runs alone
	// enough that this is reliable.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	free, err := PortAvailable(port)
	if err != nil {
		t.Fatalf("PortAvailable: %v", err)
	}
	if !free {
		t.Errorf("expected port %d to be free", port)
	}
}

func TestPortAvailable_BusyPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	free, err := PortAvailable(port)
	if err != nil {
		t.Fatalf("PortAvailable: %v", err)
	}
	if free {
		t.Errorf("expected port %d to be busy (we're still holding it)", port)
	}
}

func TestPortAvailable_OutOfRange(t *testing.T) {
	for _, p := range []int{0, -1, 65536, 100000} {
		t.Run(fmt.Sprintf("port=%d", p), func(t *testing.T) {
			_, err := PortAvailable(p)
			if err == nil {
				t.Errorf("expected error for port %d", p)
			}
		})
	}
}
