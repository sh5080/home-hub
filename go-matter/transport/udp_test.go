package transport

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUDPRoundTrip(t *testing.T) {
	a, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if err := a.SetPeer(b.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if err := b.SetPeer(a.LocalAddr()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := a.Send([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	got, err := b.Receive(ctx)
	if err != nil || string(got) != "ping" {
		t.Fatalf("b received %q (%v)", got, err)
	}

	if err := b.Send([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	got, err = a.Receive(ctx)
	if err != nil || string(got) != "pong" {
		t.Fatalf("a received %q (%v)", got, err)
	}
}

func TestUDPReceiveContextCancel(t *testing.T) {
	a, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := a.Receive(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestUDPSendWithoutPeer(t *testing.T) {
	a, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Send([]byte("x")); err == nil {
		t.Fatal("send without a peer should fail")
	}
}
