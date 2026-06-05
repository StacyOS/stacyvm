package providers

import "testing"

func TestIsLoopbackHost(t *testing.T) {
	loop := []string{"localhost", "127.0.0.1", "::1", "0.0.0.0", "127.0.0.5"}
	for _, h := range loop {
		if !isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = false, want true", h)
		}
	}
	notLoop := []string{"10.0.0.5", "example.com", "172.17.0.2", "192.168.1.1"}
	for _, h := range notLoop {
		if isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = true, want false", h)
		}
	}
}
