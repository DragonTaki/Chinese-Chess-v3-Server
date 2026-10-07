/* ----- ----- ----- ----- */
// clock.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import "time"

// clock is one side's elapsed times: used is the total time (count down: the increment is taken
// off it after each own move, uncapped, so it can go below zero, as the client's PlayerTimer), step
// the time of the current move (only measured by count-up clocks or with the step timer on).
type clock struct {
	used, step time.Duration
}

// clockState is a game's clocks: the settings, one clock per side (indexed as game.players), the
// side to move, when its move began and when each side went away (zero: connected). A plain value
// with no timers or pointers, so it can be copied as a snapshot; every method takes the current
// time instead of reading it.
type clockState struct {
	settings  TimerSettings
	clocks    []clock
	toMove    int
	turnStart time.Time
	away      []time.Time
}

// newClockState is the clocks of a game of players sides started at start, side 0 to move.
func newClockState(settings TimerSettings, players int, start time.Time) clockState {
	return clockState{
		settings:  settings,
		clocks:    make([]clock, players),
		turnStart: start,
		away:      make([]time.Time, players),
	}
}

// countDown is whether the clocks have limits (count up only measures).
func (cs *clockState) countDown() bool { return cs.settings.Mode != "CountUp" }

// measuresStep is whether the step time is measured: always counting up, with the step timer on
// counting down.
func (cs *clockState) measuresStep() bool { return !cs.countDown() || cs.settings.StepTimer }

// awayGrace is whether an away side loses by Disconnect after DisconnectGraceSeat; with count-down
// step time it simply runs out of time instead.
func (cs *clockState) awayGrace() bool { return !cs.countDown() || !cs.settings.StepTimer }

func (cs *clockState) totalLimit() time.Duration {
	return time.Duration(cs.settings.TotalMinutes) * time.Minute
}

func (cs *clockState) stepLimit() time.Duration {
	return time.Duration(cs.settings.StepSeconds) * time.Second
}

// elapsed is the time the side to move has spent on the current move so far.
func (cs *clockState) elapsed(now time.Time) time.Duration {
	if d := now.Sub(cs.turnStart); d > 0 {
		return d
	}
	return 0
}

// snapshot is every side's clock at now: the mover's includes its move in progress.
func (cs *clockState) snapshot(now time.Time) []clock {
	out := make([]clock, len(cs.clocks))
	copy(out, cs.clocks)
	d := cs.elapsed(now)
	out[cs.toMove].used += d
	if cs.measuresStep() {
		out[cs.toMove].step += d
	}
	return out
}

// expiry is the earliest game end of the clocks, whether before or after now: the losing side, the
// reason (ClockReasonTimeUp or ClockReasonDisconnect) and when. ok is false when nothing can end
// the game as things stand (count up with everyone connected). On a tie the mover's TimeUp wins.
func (cs *clockState) expiry(now time.Time) (side int, reason string, at time.Time, ok bool) {
	consider := func(s int, r string, t time.Time) {
		if !ok || t.Before(at) {
			side, reason, at, ok = s, r, t, true
		}
	}
	if cs.countDown() {
		c := cs.clocks[cs.toMove]
		consider(cs.toMove, ClockReasonTimeUp, cs.turnStart.Add(cs.totalLimit()-c.used))
		if cs.settings.StepTimer {
			consider(cs.toMove, ClockReasonTimeUp, cs.turnStart.Add(cs.stepLimit()-c.step))
		}
	}
	if cs.awayGrace() {
		for s, t := range cs.away {
			if !t.IsZero() {
				consider(s, ClockReasonDisconnect, t.Add(DisconnectGraceSeat))
			}
		}
	}
	return
}

// expired is the game end of the clocks reached by now, if any.
func (cs *clockState) expired(now time.Time) (side int, reason string, ok bool) {
	side, reason, when, ok := cs.expiry(now)
	if !ok || when.After(now) {
		return 0, "", false
	}
	return side, reason, true
}

// commitMove ends the mover's move at now (after a legal move): its time is added, the step reset,
// the increment taken off (count down) and the next side is to move.
func (cs *clockState) commitMove(now time.Time) {
	c := &cs.clocks[cs.toMove]
	c.used += cs.elapsed(now)
	c.step = 0
	if cs.countDown() {
		c.used -= time.Duration(cs.settings.IncrementSeconds) * time.Second
	}
	cs.toMove = (cs.toMove + 1) % len(cs.clocks)
	cs.turnStart = now
}

// setAway marks side as away from now; a side already away keeps its first time.
func (cs *clockState) setAway(side int, now time.Time) {
	if cs.away[side].IsZero() {
		cs.away[side] = now
	}
}

// setBack marks side as connected again.
func (cs *clockState) setBack(side int, _ time.Time) {
	cs.away[side] = time.Time{}
}
