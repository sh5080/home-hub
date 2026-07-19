package ash

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.bug.st/serial"
)

// BaudRate is the ASH serial line speed for EmberZNet NCPs.
const BaudRate = 115200

// Conn is an ASH connection over a serial port. It runs a background reader that
// frames incoming bytes, dispatches control frames, auto-acknowledges DATA
// frames, and delivers their EZSP payloads on Recv. Send wraps an EZSP payload
// in a DATA frame. This is a minimal ASH implementation sufficient for the EZSP
// command/response flow; the full sliding-window retransmit is a TODO.
type Conn struct {
	port serial.Port
	log  func(string, ...any)

	mu    sync.Mutex
	txSeq uint8 // next DATA frame number to send (0-7)
	rxSeq uint8 // next expected DATA frame number (0-7)

	acks    chan uint8  // ack numbers from ACK/NAK frames
	rstacks chan byte   // RSTACK reset codes
	data    chan []byte // received EZSP payloads
	errs    chan error
}

// Open opens the serial port and starts the ASH reader.
func Open(portName string, log func(string, ...any)) (*Conn, error) {
	p, err := serial.Open(portName, &serial.Mode{BaudRate: BaudRate})
	if err != nil {
		return nil, fmt.Errorf("ash open %s: %w", portName, err)
	}
	c := &Conn{
		port: p, log: log,
		acks: make(chan uint8, 8), rstacks: make(chan byte, 1),
		data: make(chan []byte, 32), errs: make(chan error, 1),
	}
	go c.read()
	return c, nil
}

// Close closes the serial port.
func (c *Conn) Close() error { return c.port.Close() }

// read is the framing loop: accumulate bytes until a flag, then decode.
func (c *Conn) read() {
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 256)
	for {
		n, err := c.port.Read(tmp)
		if err != nil {
			select {
			case c.errs <- err:
			default:
			}
			return
		}
		for _, b := range tmp[:n] {
			switch b {
			case cancel:
				buf = buf[:0] // discard partial frame
			case Flag:
				if len(buf) > 0 {
					c.dispatch(buf)
					buf = buf[:0]
				}
			default:
				buf = append(buf, b)
			}
		}
	}
}

func (c *Conn) dispatch(raw []byte) {
	f, err := Decode(raw)
	if err != nil {
		if c.log != nil {
			c.log("ash decode", "err", err)
		}
		return
	}
	switch f.Type {
	case RSTACK:
		select {
		case c.rstacks <- f.Code:
		default:
		}
	case ACK:
		select {
		case c.acks <- f.AckNum:
		default:
		}
	case NAK:
		// Minimal: surface as an ack so the sender unblocks; retransmit TODO.
		select {
		case c.acks <- f.AckNum:
		default:
		}
	case ERROR:
		select {
		case c.errs <- fmt.Errorf("ash: NCP ERROR code %#02x", f.Code):
		default:
		}
	case DATA:
		c.mu.Lock()
		c.rxSeq = (f.FrmNum + 1) & 0x07
		ack := c.rxSeq
		c.mu.Unlock()
		_, _ = c.port.Write(AckFrame(ack)) // acknowledge receipt
		// A DATA frame also piggybacks an ack for our sent frames; surface it so
		// a pending Send unblocks even when the NCP never sends a standalone ACK.
		select {
		case c.acks <- f.AckNum:
		default:
		}
		select {
		case c.data <- f.Payload:
		default:
		}
	}
}

// Reset sends an ASH RST and waits for the NCP's RSTACK, establishing the link.
func (c *Conn) Reset(ctx context.Context) error {
	// Prefix Cancel to flush any partial frame the NCP may be mid-parse on.
	if _, err := c.port.Write(append([]byte{cancel}, RstFrame()...)); err != nil {
		return err
	}
	c.mu.Lock()
	c.txSeq, c.rxSeq = 0, 0
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c.errs:
		return err
	case code := <-c.rstacks:
		if c.log != nil {
			c.log("ash reset ack", "code", fmt.Sprintf("%#02x", code))
		}
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("ash: no RSTACK (reset timeout)")
	}
}

// Send frames an EZSP payload in a DATA frame and waits for its ACK.
func (c *Conn) Send(ctx context.Context, ezsp []byte) error {
	c.mu.Lock()
	frm, ack := c.txSeq, c.rxSeq
	c.txSeq = (c.txSeq + 1) & 0x07
	c.mu.Unlock()
	if _, err := c.port.Write(DataFrame(frm, ack, ezsp)); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c.errs:
		return err
	case <-c.acks:
		return nil
	case <-time.After(2 * time.Second):
		return errors.New("ash: no ACK (send timeout)")
	}
}

// Recv returns the next received EZSP payload.
func (c *Conn) Recv(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-c.errs:
		return nil, err
	case p := <-c.data:
		return p, nil
	}
}
