package session

import "errors"

// errReplay indicates a message counter that has already been accepted or is
// older than the replay window.
var errReplay = errors.New("session: replayed or stale message counter")

// replayWindow rejects duplicate and stale message counters (Spec 4.6.7). It
// keeps the highest counter accepted and a 32-bit bitmap of the counters
// immediately below it.
type replayWindow struct {
	max    uint32
	bitmap uint32
	init   bool
}

// accept records counter c, returning errReplay if it is a duplicate or too old.
func (w *replayWindow) accept(c uint32) error {
	if !w.init {
		w.init = true
		w.max = c
		return nil
	}
	switch {
	case c > w.max:
		shift := c - w.max
		if shift < 32 {
			w.bitmap = (w.bitmap << shift) | (1 << (shift - 1))
		} else {
			w.bitmap = 0
		}
		w.max = c
		return nil
	case c == w.max:
		return errReplay
	default:
		diff := w.max - c
		if diff > 32 {
			return errReplay // older than the window
		}
		bit := uint32(1) << (diff - 1)
		if w.bitmap&bit != 0 {
			return errReplay
		}
		w.bitmap |= bit
		return nil
	}
}
