package tailnet

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestRequireRejectsUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	ln.Close()

	err = Require(context.Background(), addr, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), ErrNotOnTailnet) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
