package transport

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPipeRoundTrip(t *testing.T) {
	a, b := NewPipe()
	defer a.Close()

	if err := a.Send([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, err := b.Receive(context.Background())
	if err != nil || string(got) != "hello" {
		t.Fatalf("b received %q (%v)", got, err)
	}

	if err := b.Send([]byte("world")); err != nil {
		t.Fatal(err)
	}
	got, err = a.Receive(context.Background())
	if err != nil || string(got) != "world" {
		t.Fatalf("a received %q (%v)", got, err)
	}
}

func TestPipeSendCopies(t *testing.T) {
	a, b := NewPipe()
	buf := []byte("data")
	a.Send(buf)
	buf[0] = 'X' // mutate after send; the received copy must be unaffected
	got, _ := b.Receive(context.Background())
	if string(got) != "data" {
		t.Fatalf("received %q, sender mutation leaked", got)
	}
}

func TestPipeReceiveContext(t *testing.T) {
	a, _ := NewPipe()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := a.Receive(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestPipeClose(t *testing.T) {
	a, b := NewPipe()
	a.Close()
	if err := a.Send([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("send after close: %v", err)
	}
	if _, err := b.Receive(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("receive after close: %v", err)
	}
	a.Close() // idempotent
}
