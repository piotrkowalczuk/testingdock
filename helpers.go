package testingdock

import (
	"fmt"
	"net"
	"strconv"
	"testing"
)

// printf just wraps fmt.Printf.
func printf(format string, args ...interface{}) {
	fmt.Printf("··· DOCK: %s\n", fmt.Sprintf(format, args...))
}

// RandomPort returns a random available port as a string.
func RandomPort(t testing.TB) string {
	return strconv.FormatInt(int64(randomPort(t)), 10)

}

// randomPort returns random available port as an int.
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

// Check whether a map containing labels has the "owner=testingdock" label.
func isOwnedByTestingdock(labels map[string]string) bool {
	for key, value := range labels {
		if fmt.Sprintf("%s=%s", key, value) == "owner=testingdock" {
			return true
		}
	}
	return false
}

// Create a map of labels containting the "owner=testingdock" label.
func createTestingLabel() map[string]string {
	labels := make(map[string]string)
	labels["owner"] = "testingdock"
	return labels
}
