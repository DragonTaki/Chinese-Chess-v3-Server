/* ----- ----- ----- ----- */
// pending.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"sync"
	"time"
)

const (
	pendingResultTTL = 10 * time.Minute // how long a missed game result waits for its player to log in again
	pendingResultMax = 1000             // most results kept (the oldest goes first)
)

// pendingResult is the EndGame of a game that ended while the player was away.
type pendingResult struct {
	data   EndGameData
	roomId string
	at     time.Time
}

// pendingResults holds, in memory only, the result of the game each away player missed, by account
// id, until the player logs in again (it is then delivered once) or pendingResultTTL passes.
type pendingResults struct {
	mu   sync.Mutex
	list map[string]pendingResult
}

func newPendingResults() *pendingResults {
	return &pendingResults{list: make(map[string]pendingResult)}
}

// put remembers the result for uid (replacing an older one); the oldest entries go when full.
func (p *pendingResults) put(uid, roomId string, data EndGameData, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, r := range p.list {
		if now.Sub(r.at) > pendingResultTTL {
			delete(p.list, id)
		}
	}
	for len(p.list) >= pendingResultMax {
		oldest, first := "", true
		var at time.Time
		for id, r := range p.list {
			if first || r.at.Before(at) {
				oldest, at, first = id, r.at, false
			}
		}
		delete(p.list, oldest)
	}
	p.list[uid] = pendingResult{data: data, roomId: roomId, at: now}
}

// take removes and returns the result waiting for uid (false: none, or it expired).
func (p *pendingResults) take(uid string, now time.Time) (pendingResult, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.list[uid]
	delete(p.list, uid)
	if !ok || now.Sub(r.at) > pendingResultTTL {
		return pendingResult{}, false
	}
	return r, true
}
