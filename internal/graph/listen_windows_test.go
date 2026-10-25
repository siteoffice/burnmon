//go:build windows

package graph

import (
	"net"
	"os"
	"testing"
)

// A socket this test opens shows in the table with this process's pid.
func TestListeningSeesOwnSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	got, err := Listening()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got {
		if l.Port == port && l.PID == int32(os.Getpid()) && l.Proto == "tcp" {
			return
		}
	}
	t.Fatalf("own listening port %d not found among %d rows", port, len(got))
}
