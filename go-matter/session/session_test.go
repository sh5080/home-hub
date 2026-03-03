package session

import (
	"bytes"
	"testing"

	"github.com/sh5080/go-matter/message"
)

func newPair(t *testing.T) (ctrl, dev *Secure) {
	t.Helper()
	i2r := bytes.Repeat([]byte{0x11}, 16)
	r2i := bytes.Repeat([]byte{0x22}, 16)
	c, err := NewSecure(1, 2, 100, 200, i2r, r2i, 0)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewSecure(2, 1, 200, 100, r2i, i2r, 0)
	if err != nil {
		t.Fatal(err)
	}
	return c, d
}

func TestSessionRoundTrip(t *testing.T) {
	ctrl, dev := newPair(t)

	msg := []byte("invoke window-covering go-to 37%")
	frame, err := ctrl.Encrypt(msg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := dev.Decrypt(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("device got %q", got)
	}

	reply := []byte("status success")
	frame2, err := dev.Encrypt(reply)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := ctrl.Decrypt(frame2)
	if err != nil || !bytes.Equal(got2, reply) {
		t.Fatalf("ctrl got %q (%v)", got2, err)
	}
}

func TestSessionTamperAndMisroute(t *testing.T) {
	ctrl, dev := newPair(t)
	frame, _ := ctrl.Encrypt([]byte("secret"))

	bad := append([]byte(nil), frame...)
	bad[len(bad)-1] ^= 1
	if _, err := dev.Decrypt(bad); err == nil {
		t.Fatal("tampered frame accepted")
	}
	// The sender cannot decrypt its own message (wrong session id and keys).
	if _, err := ctrl.Decrypt(frame); err == nil {
		t.Fatal("misrouted frame accepted")
	}
}

func TestSessionCounterAdvances(t *testing.T) {
	ctrl, dev := newPair(t)
	f1, _ := ctrl.Encrypt([]byte("a"))
	f2, _ := ctrl.Encrypt([]byte("b"))

	h1, _, _ := message.Decode(f1)
	h2, _, _ := message.Decode(f2)
	if h1.Counter == h2.Counter {
		t.Fatal("transmit counter did not advance")
	}
	if _, err := dev.Decrypt(f1); err != nil {
		t.Fatal(err)
	}
	if _, err := dev.Decrypt(f2); err != nil {
		t.Fatal(err)
	}
}
