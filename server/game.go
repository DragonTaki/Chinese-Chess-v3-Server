/* ----- ----- ----- ----- */
// game.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"sync"
	"time"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server/db"
	"Chinese-Chess-v3-Server/server/rules"
)

// An online game is server-authoritative (ANTI-CHEAT.md): the server keeps the position, checks
// every action with the rules host, decides the end and records the game; a client only sends what
// it wants to do. Traditional (Full board) only so far; clocks come with the next step.

// Game error codes, sent as a rejected GameUpdate's reason or an Error packet's data.
const (
	GameErrNoGame           = "NoGame"
	GameErrNotYourTurn      = "NotYourTurn"
	GameErrRulesUnavailable = "RulesUnavailable"
)

// rulesTimeout bounds one rules host call.
const rulesTimeout = 5 * time.Second

// moveEntry is one move of the game's record.
type moveEntry struct {
	From     [2]int `json:"from"`
	To       [2]int `json:"to"`
	Notation string `json:"notation"`
}

// game is a room's game in progress. Guarded by its own lock: actions of one game are handled one
// at a time, games of different rooms in parallel.
type game struct {
	mu        sync.Mutex
	id        string
	room      *Room
	kind      string
	rules     map[string]bool
	players   []*Client // in turn order: players[0] is Player1 (red, moves first)
	position  rules.Position
	history   rules.History
	moves     []moveEntry
	startedAt time.Time
	over      bool
}

// sideOf is c's player number (1, 2); 0 when c does not play in g.
func (g *game) sideOf(c *Client) int {
	for i, p := range g.players {
		if p == c {
			return i + 1
		}
	}
	return 0
}

// randomBool is a fair coin from crypto/rand.
func randomBool() bool {
	n, err := rand.Int(rand.Reader, big.NewInt(2))
	if err != nil {
		panic(err)
	}
	return n.Int64() == 1
}

// standardPosition is the Traditional start, red (Player1) to move: FEN coordinates, red at the
// bottom (y 9), black at the top (y 0).
func standardPosition() rules.Position {
	back := []string{"Chariot", "Horse", "Elephant", "Advisor", "General", "Advisor", "Elephant", "Horse", "Chariot"}
	var pieces []rules.Piece
	add := func(t, color string, side, x, y int) {
		pieces = append(pieces, rules.Piece{Type: t, Color: color, Side: side, X: x, Y: y, FaceUp: true})
	}
	for x, t := range back {
		add(t, "Black", 2, x, 0)
		add(t, "Red", 1, x, 9)
	}
	for _, x := range []int{1, 7} {
		add("Cannon", "Black", 2, x, 2)
		add("Cannon", "Red", 1, x, 7)
	}
	for _, x := range []int{0, 2, 4, 6, 8} {
		add("Soldier", "Black", 2, x, 3)
		add("Soldier", "Red", 1, x, 6)
	}
	return rules.Position{ToMove: 1, Pieces: pieces}
}

// StartGameData is what each player receives when the game starts.
type StartGameData struct {
	GameId   string          `json:"gameId"`
	Kind     string          `json:"kind"`
	Rules    map[string]bool `json:"rules"`
	Timer    TimerSettings   `json:"timer"`
	Players  []SeatView      `json:"players"` // in turn order
	YourSide int             `json:"yourSide"`
	Position rules.Position  `json:"position"`
}

// GameUpdateData is a validated action as every player sees it, or (Rejected set) an action of the
// receiver's that was refused.
type GameUpdateData struct {
	Seq      int             `json:"seq"`
	Rejected string          `json:"rejected,omitempty"`
	Ply      int             `json:"ply,omitempty"`
	Mover    int             `json:"mover,omitempty"`
	Record   *rules.Record   `json:"record,omitempty"`
	Position *rules.Position `json:"position,omitempty"`
	Check    bool            `json:"check,omitempty"`
}

// EndGameData is how the game ended: the winner (player number; 0 for a draw) and why.
type EndGameData struct {
	Winner int    `json:"winner"`
	Reason string `json:"reason"`
}

// GameActionData is what a player wants to do: Type "move" (From, To) or "resign".
type GameActionData struct {
	Seq  int     `json:"seq"`
	Type string  `json:"type"`
	From *[2]int `json:"from,omitempty"`
	To   *[2]int `json:"to,omitempty"`
}

// startGame starts r's game once every seat is ready: the first mover from the room's settings,
// the start position, StartGame to each player. Called without the room manager's lock.
func (s *Server) startGame(r *Room) {
	if s.rules == nil {
		for _, c := range s.rooms.Members(r) {
			sendRoomError(c, GameErrRulesUnavailable, "")
		}
		return
	}
	players, ok := s.rooms.BeginGame(r)
	if !ok {
		return
	}
	// Seat 0 is the host's seat when the host created the room; "Host" / "Guest" pick by the host.
	host := s.rooms.HostOf(r)
	first := players[0]
	switch r.Settings.FirstMover {
	case "Host":
		first = host
	case "Guest":
		for _, p := range players {
			if p != host {
				first = p
			}
		}
	default:
		if randomBool() {
			first = players[1]
		}
	}
	ordered := []*Client{first}
	for _, p := range players {
		if p != first {
			ordered = append(ordered, p)
		}
	}

	g := &game{
		id: s.rooms.NewGameId(), room: r, kind: r.Settings.Kind, rules: r.Settings.Rules,
		players: ordered, position: standardPosition(), startedAt: time.Now(),
	}
	s.rooms.SetGame(r, g)

	views := make([]SeatView, len(ordered))
	for i, p := range ordered {
		views[i] = SeatView{Id: p.Id(), Name: p.Name()}
	}
	for i, p := range ordered {
		data, _ := json.Marshal(StartGameData{
			GameId: g.id, Kind: g.kind, Rules: g.rules, Timer: r.Settings.Timer,
			Players: views, YourSide: i + 1, Position: g.position,
		})
		p.SendPacket(CreatePacket(PacketTypeStartGame, "Server", r.Id, string(data), ""))
	}
	s.sendRoomState(r)
	logger.Infof("Game %s started in room %s", g.id, r.Id)
}

// handleGameAction checks and applies a player's action in its room's game.
func (s *Server) handleGameAction(c *Client, pkt *Packet) {
	var act GameActionData
	if err := json.Unmarshal([]byte(pkt.Data), &act); err != nil {
		sendRoomError(c, RoomErrInvalidData, err.Error())
		return
	}
	g := s.rooms.GameOf(c)
	if g == nil {
		sendRoomError(c, GameErrNoGame, "")
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.over {
		s.reject(c, act.Seq, GameErrNoGame)
		return
	}
	side := g.sideOf(c)
	switch act.Type {
	case "resign":
		s.endGame(g, 3-side, "Resign")
	case "move":
		if side != g.position.ToMove {
			s.reject(c, act.Seq, GameErrNotYourTurn)
			return
		}
		if act.From == nil || act.To == nil {
			s.reject(c, act.Seq, RoomErrInvalidData)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), rulesTimeout)
		defer cancel()
		res, err := s.rules.Validate(ctx, rules.Game{Kind: g.kind, Rules: hostRules(g.rules), Position: g.position},
			rules.MoveAction(act.From[0], act.From[1], act.To[0], act.To[1]), &g.history)
		if err != nil {
			logger.Errorf("Game %s: the rules host failed: %v", g.id, err)
			s.reject(c, act.Seq, GameErrRulesUnavailable)
			return
		}
		if !res.Legal {
			s.reject(c, act.Seq, res.Reason)
			return
		}
		g.position = *res.Position
		g.moves = append(g.moves, moveEntry{From: *act.From, To: *act.To, Notation: res.Record.Notation})
		move := rules.Move{From: *act.From, To: *act.To}
		if res.Record.Captured != nil {
			g.history = rules.History{}
		} else {
			g.history.PliesSinceCapture++
			g.history.RecentMoves = append(g.history.RecentMoves, move)
		}
		update := GameUpdateData{Seq: act.Seq, Ply: len(g.moves), Mover: side, Record: res.Record, Position: &g.position, Check: res.Check}
		data, _ := json.Marshal(update)
		for _, p := range g.players {
			p.SendPacket(CreatePacket(PacketTypeGameUpdate, "Server", g.room.Id, string(data), ""))
		}
		if res.GameOver != nil {
			s.endGame(g, res.GameOver.Winner, res.GameOver.Reason)
		}
	default:
		s.reject(c, act.Seq, RoomErrInvalidData)
	}
}

// reject tells c its action seq was refused (only c sees it).
func (s *Server) reject(c *Client, seq int, reason string) {
	data, _ := json.Marshal(GameUpdateData{Seq: seq, Rejected: reason})
	c.SendPacket(CreatePacket(PacketTypeGameUpdate, "Server", "", string(data), ""))
}

// endGame ends g (its lock held): EndGame to the players, the record written, the room back to
// waiting (everyone's ready cleared). winner is a player number, 0 for a draw.
func (s *Server) endGame(g *game, winner int, reason string) {
	g.over = true
	data, _ := json.Marshal(EndGameData{Winner: winner, Reason: reason})
	for _, p := range g.players {
		p.SendPacket(CreatePacket(PacketTypeEndGame, "Server", g.room.Id, string(data), ""))
	}

	ids := make([]string, len(g.players))
	for i, p := range g.players {
		ids[i] = p.Id()
	}
	record := &db.Game{
		ID: g.id, Kind: g.kind, Mode: g.room.Settings.Mode, Players: mustJSON(ids), Reason: reason,
		Moves: mustJSON(g.moves), StartedAt: g.startedAt, EndedAt: time.Now(),
	}
	settings := g.room.Settings
	record.Rules = mustJSON(struct {
		Rules map[string]bool `json:"rules"`
		Timer TimerSettings   `json:"timer"`
	}{settings.Rules, settings.Timer})
	if winner > 0 {
		record.Winner = ids[winner-1]
	}
	if err := db.SaveGame(s.dbConn, record); err != nil {
		logger.Errorf("Game %s could not be saved: %v", g.id, err)
	}
	s.rooms.EndGame(g.room)
	s.sendRoomState(g.room)
	logger.Infof("Game %s ended: winner %d (%s)", g.id, winner, reason)
}

// hostRules is a room's rule switches as the rules host takes them (nil: the defaults).
func hostRules(switches map[string]bool) map[string]any {
	if len(switches) == 0 {
		return nil
	}
	m := make(map[string]any, len(switches))
	for k, v := range switches {
		m[k] = v
	}
	return m
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
