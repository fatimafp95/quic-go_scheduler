package utils

import (
	"math"
	"time"
)

// A Timer wrapper that behaves correctly when resetting
type Timer struct {
	T    *time.Timer
	read bool
	deadline time.Time
}

// NewTimer creates a new timer that is not set
func NewTimer() *Timer {
	return &Timer{T: time.NewTimer(time.Duration(math.MaxInt64))}
}

// Chan returns the channel of the wrapped timer
func (t *Timer) Chan() <-chan time.Time {
	return t.T.C
}

// Reset the timer, no matter whether the value was read or not
func (t *Timer) Reset(deadline time.Time) {
	if deadline.Equal(t.deadline) && !t.read {
		// No need to reset the timer
		return
	}

	// We need to drain the timer if the value from its channel was not read yet.
	// See https://groups.google.com/forum/#!topic/golang-dev/c9UUfASVPoU
	if !t.T.Stop() && !t.read {
		<-t.T.C
	}
	if !deadline.IsZero() {
		t.T.Reset(time.Until(deadline))
	}

	t.read = false
	t.deadline = deadline
}

// SetRead should be called after the value from the chan was read
func (t *Timer) SetRead() {
	t.read = true
}

// Stop stops the timer
func (t *Timer) Stop() {
	t.T.Stop()
}
