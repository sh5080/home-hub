package controller

import (
	"context"
	"testing"
	"time"
)

// TestDialAddrUnreachable checks that dialing a device with no responder fails
// (rather than hanging), exercising the UDP-open + CASE + timeout path.
func TestDialAddrUnreachable(t *testing.T) {
	fabric, ctrlID, _ := buildFabric(t)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	sess, err := New(fabric, ctrlID).DialAddr(ctx, devNode, "127.0.0.1:59991")
	if err == nil {
		sess.Close()
		t.Fatal("expected an error dialing an unreachable device")
	}
}
