package testingdock

import (
	"fmt"
	"net"
	"strconv"
	"testing"
)

func printf(format string, args ...interface{}) {
	fmt.Printf("··· DOCK: %s\n", fmt.Sprintf(format, args...))
}

// RandomPort ...
func RandomPort(t testing.TB) string {
	return strconv.FormatInt(int64(randomPort(t)), 10)

}

// RandomPort returns random available port.
func randomPort(t testing.TB) int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("testingdock: resolve failure: %s", err.Error())
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatalf("testingdock: listen failure: %s", err.Error())
	}

	if err := l.Close(); err != nil {
		t.Fatalf("testingdock: listener closing failure: %s", err.Error())
	}

	return l.Addr().(*net.TCPAddr).Port
}
