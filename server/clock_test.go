/* ----- ----- ----- ----- */
// clock_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

func countDown(total, step, inc int, stepTimer bool) TimerSettings {
	return TimerSettings{Mode: "CountDown", TotalMinutes: total, StepSeconds: step, StepTimer: stepTimer, IncrementSeconds: inc}
}

var countUp = TimerSettings{Mode: "CountUp", TotalMinutes: 10, StepSeconds: 30, StepTimer: true, IncrementSeconds: 5}

const sec = time.Second

func TestClockExpiry(t *testing.T) {
	type want struct {
		side   int
		reason string
		at     time.Time
		ok     bool
	}
	tests := []struct {
		name  string
		setup func() clockState
		want  want
	}{
		{"count down step expiry", func() clockState {
			return newClockState(countDown(10, 30, 0, true), 2, t0)
		}, want{0, ClockReasonTimeUp, at(30 * sec), true}},
		{"count down total expiry before step", func() clockState {
			cs := newClockState(countDown(1, 3600, 0, true), 2, t0)
			return cs
		}, want{0, ClockReasonTimeUp, at(time.Minute), true}},
		{"step before the remaining total", func() clockState {
			cs := newClockState(countDown(1, 30, 0, true), 2, t0)
			cs.commitMove(at(20 * sec)) // P1 used 20s
			cs.commitMove(at(25 * sec)) // P2 used 5s
			return cs                   // P1: 40s left, step 30s: the step runs out first
		}, want{0, ClockReasonTimeUp, at(55 * sec), true}},
		{"total expiry with less left than step", func() clockState {
			cs := newClockState(countDown(1, 30, 0, true), 2, t0)
			cs.commitMove(at(29 * sec))
			cs.commitMove(at(30 * sec))
			cs.commitMove(at(59 * sec)) // P1 used 58s
			cs.commitMove(at(60 * sec))
			return cs // P1: 2s left
		}, want{0, ClockReasonTimeUp, at(62 * sec), true}},
		{"step timer off: no step expiry", func() clockState {
			return newClockState(countDown(10, 30, 0, false), 2, t0)
		}, want{0, ClockReasonTimeUp, at(10 * time.Minute), true}},
		{"count up connected never expires", func() clockState {
			return newClockState(countUp, 2, t0)
		}, want{}},
		{"count up away mover: Disconnect after grace", func() clockState {
			cs := newClockState(countUp, 2, t0)
			cs.setAway(0, at(100*sec))
			return cs
		}, want{0, ClockReasonDisconnect, at(115 * sec), true}},
		{"count down step off away: Disconnect after grace", func() clockState {
			cs := newClockState(countDown(10, 30, 0, false), 2, t0)
			cs.setAway(0, at(5*sec))
			return cs
		}, want{0, ClockReasonDisconnect, at(20 * sec), true}},
		{"count down step on away: TimeUp, no Disconnect", func() clockState {
			cs := newClockState(countDown(10, 30, 0, true), 2, t0)
			cs.setAway(0, at(1*sec))
			return cs
		}, want{0, ClockReasonTimeUp, at(30 * sec), true}},
		{"count down step on away non-mover: mover's TimeUp only", func() clockState {
			cs := newClockState(countDown(10, 30, 0, true), 2, t0)
			cs.setAway(1, at(1*sec))
			return cs
		}, want{0, ClockReasonTimeUp, at(30 * sec), true}},
		{"away non-mover: Disconnect before mover's total", func() clockState {
			cs := newClockState(countDown(10, 30, 0, false), 2, t0)
			cs.setAway(1, at(2*sec))
			return cs
		}, want{1, ClockReasonDisconnect, at(17 * sec), true}},
		{"away but total runs out first: TimeUp", func() clockState {
			cs := newClockState(countDown(1, 30, 0, false), 2, t0)
			cs.setAway(0, at(50*sec))
			return cs
		}, want{0, ClockReasonTimeUp, at(60 * sec), true}},
		{"setBack clears grace", func() clockState {
			cs := newClockState(countUp, 2, t0)
			cs.setAway(1, at(1*sec))
			cs.setBack(1, at(3*sec))
			return cs
		}, want{}},
		{"setAway twice keeps first time", func() clockState {
			cs := newClockState(countUp, 2, t0)
			cs.setAway(1, at(1*sec))
			cs.setAway(1, at(9*sec))
			return cs
		}, want{1, ClockReasonDisconnect, at(16 * sec), true}},
		{"earliest of several aways", func() clockState {
			cs := newClockState(countUp, 2, t0)
			cs.setAway(0, at(10*sec))
			cs.setAway(1, at(4*sec))
			return cs
		}, want{1, ClockReasonDisconnect, at(19 * sec), true}},
		{"tie: mover's TimeUp wins", func() clockState {
			cs := newClockState(countDown(1, 30, 0, false), 2, t0)
			cs.setAway(1, at(45*sec))
			return cs
		}, want{0, ClockReasonTimeUp, at(60 * sec), true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := tt.setup()
			side, reason, when, ok := cs.expiry(at(time.Hour))
			got := want{side, reason, when, ok}
			if !ok {
				got = want{}
			}
			if got != tt.want {
				t.Errorf("expiry = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClockExpired(t *testing.T) {
	cs := newClockState(countDown(10, 30, 0, true), 2, t0)
	if _, _, ok := cs.expired(at(29 * sec)); ok {
		t.Fatal("expired before the step limit")
	}
	side, reason, ok := cs.expired(at(30 * sec))
	if !ok || side != 0 || reason != ClockReasonTimeUp {
		t.Fatalf("expired at the step limit = %d %q %v", side, reason, ok)
	}
	// A committed move resets the step: the next mover gets its full step.
	cs.commitMove(at(29 * sec))
	if _, _, ok := cs.expired(at(58 * sec)); ok {
		t.Fatal("expired before the second mover's step limit")
	}
	if side, _, ok := cs.expired(at(59 * sec)); !ok || side != 1 {
		t.Fatalf("second mover not expired at its step limit: %d %v", side, ok)
	}
}

func TestClockIncrement(t *testing.T) {
	cs := newClockState(countDown(10, 30, 10, true), 2, t0)
	// No increment before the move is committed.
	if got := cs.snapshot(at(3 * sec))[0].used; got != 3*sec {
		t.Fatalf("used before commit = %v, want 3s", got)
	}
	cs.commitMove(at(3 * sec))
	// Uncapped: 3s used - 10s increment = -7s, as the client's PlayerTimer.
	if got := cs.clocks[0].used; got != -7*sec {
		t.Fatalf("used after commit = %v, want -7s", got)
	}
	if cs.clocks[1].used != 0 {
		t.Fatalf("the other side got the increment: %v", cs.clocks[1].used)
	}
	cs.commitMove(at(5 * sec))
	// P1's total deadline moves out by the 7s it is ahead.
	_, _, when, _ := cs.expiry(at(5 * sec))
	if want := at(5*sec + 30*sec); !when.Equal(want) { // step still earlier
		t.Fatalf("expiry = %v, want %v", when, want)
	}
	cs.settings.StepTimer = false
	if _, _, when, _ := cs.expiry(at(5 * sec)); !when.Equal(at(5*sec + 10*time.Minute + 7*sec)) {
		t.Fatalf("total expiry = %v", when)
	}

	// Count up: no increment.
	up := newClockState(countUp, 2, t0)
	up.commitMove(at(3 * sec))
	if got := up.clocks[0].used; got != 3*sec {
		t.Fatalf("count up used after commit = %v, want 3s", got)
	}
}

func TestClockSnapshot(t *testing.T) {
	cs := newClockState(countDown(10, 30, 0, true), 2, t0)
	cs.commitMove(at(4 * sec))
	snap := cs.snapshot(at(10 * sec))
	if snap[0] != (clock{used: 4 * sec}) {
		t.Errorf("waiting side = %+v, want used 4s step 0", snap[0])
	}
	if snap[1] != (clock{used: 6 * sec, step: 6 * sec}) {
		t.Errorf("mover = %+v, want used 6s step 6s", snap[1])
	}
	if cs.clocks[1] != (clock{}) {
		t.Errorf("snapshot changed the state: %+v", cs.clocks[1])
	}

	// Step timer off (count down): the step is not measured, as the client.
	off := newClockState(countDown(10, 30, 0, false), 2, t0)
	if got := off.snapshot(at(5 * sec))[0]; got != (clock{used: 5 * sec}) {
		t.Errorf("step off mover = %+v, want used 5s step 0", got)
	}
	// Count up always measures the step.
	up := newClockState(countUp, 2, t0)
	if got := up.snapshot(at(5 * sec))[0]; got != (clock{used: 5 * sec, step: 5 * sec}) {
		t.Errorf("count up mover = %+v, want used 5s step 5s", got)
	}
}

func TestClockSideCount(t *testing.T) {
	cs := newClockState(countUp, 3, t0)
	for i, want := range []int{1, 2, 0} {
		cs.commitMove(at(time.Duration(i+1) * sec))
		if cs.toMove != want {
			t.Fatalf("after move %d toMove = %d, want %d", i+1, cs.toMove, want)
		}
	}
}
