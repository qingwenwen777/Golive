package chatlimit

import "time"

// SetNow replaces the clock that picks the window, so tests don't depend on
// where the wall clock falls relative to a window boundary.
func (l *Limiter) SetNow(now func() time.Time) { l.now = now }
