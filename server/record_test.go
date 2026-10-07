/* ----- ----- ----- ----- */
// record_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"encoding/json"
	"testing"
	"time"

	"Chinese-Chess-v3-Server/server/db"
)

// A game's record has each move's time (before the increment) and the clocks at its end, whether it
// ends by resignation or by time-up.
func TestGameRecordHasMoveTimesAndClocks(t *testing.T) {
	for _, reason := range []string{"Resign", ClockReasonTimeUp} {
		t.Run(reason, func(t *testing.T) {
			s := testServer(t)
			g, _, _, _, _ := runningGame(t, s, countDown(10, 30, 5, false))
			g.mu.Lock()
			start := g.clocks.turnStart
			g.commitMove([2]int{7, 7}, [2]int{4, 7}, "m1", start.Add(1500*time.Millisecond))
			g.commitMove([2]int{7, 0}, [2]int{6, 2}, "m2", start.Add(1500*time.Millisecond+400*time.Millisecond))
			g.commitMove([2]int{4, 7}, [2]int{4, 3}, "m3", start.Add(1500*time.Millisecond+400*time.Millisecond-time.Second)) // clock going back: 0
			s.endGame(g, 2, reason)
			g.mu.Unlock()

			var rec db.Game
			if err := s.dbConn.First(&rec, "id = ?", "g").Error; err != nil {
				t.Fatal(err)
			}
			var moves []moveEntry
			if err := json.Unmarshal([]byte(rec.Moves), &moves); err != nil || len(moves) != 3 {
				t.Fatalf("moves %q (%v)", rec.Moves, err)
			}
			if moves[0].Ms != 1500 || moves[1].Ms != 400 || moves[2].Ms != 0 {
				t.Fatalf("ms %+v", moves)
			}
			var ck ClocksData
			if err := json.Unmarshal([]byte(rec.Clocks), &ck); err != nil || len(ck.Sides) != 2 {
				t.Fatalf("clocks %q (%v)", rec.Clocks, err)
			}
			// Player1 moved twice (1500 and 0 ms, less 5 s each); Player2 moved once (400 ms, less 5 s)
			// and is to move, its move in progress counted.
			if ck.Sides[0].UsedMs != 1500-10000 || ck.Sides[1].UsedMs < 400-5000 || ck.Sides[1].UsedMs > 400-5000+5000 || ck.ToMove != 1 {
				t.Fatalf("clocks %+v", ck)
			}
		})
	}
}
