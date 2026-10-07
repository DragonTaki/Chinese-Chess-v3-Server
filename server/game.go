/* ----- ----- ----- ----- */
// game.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.2
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
// it wants to do. Traditional (Full board) only so far. The clocks are the server's too: a move is
// timed from when its packet is received, and the game ends by TimeUp when the mover's time runs
// out (a deadline timer per game). A player whose connection ends during the game keeps its seat
// and is away: its clocks keep running, and (as clockState.expiry decides) it loses by TimeUp when
// its step time runs out, or by Disconnect after DisconnectGraceSeat where no step time would end
// the game. An explicit LeaveRoom still resigns.

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
	Ms       int64  `json:"ms"` // the time the mover spent on the move (before the increment)
}

// game is a room's game in progress. Guarded by its own lock: actions of one game are handled one
// at a time, games of different rooms in parallel.
type game struct {
	mu        sync.Mutex
	id        string
	room      *Room
	kind      string
	rules     map[string]bool
	players   []player // in turn order: players[0] is Player1 (red, moves first)
	position  rules.Position
	history   rules.History
	moves     []moveEntry
	startedAt time.Time
	over      bool
	clocks    clockState
	deadline  *time.Timer   // fires at the clocks' next expiry; nil when none is armed
	gen       uint64        // bumped on every re-arm and at the end: a stale deadline callback does nothing
	done      chan struct{} // closed by endGame (once): stops the game's TimerSync goroutine
}

// player is one player of a game: the account, and the connection its packets go to (swapped by
// rebind).
type player struct {
	id   string
	conn *Client
}

// ClockView is one side's clock as sent: the total time used and the current move's time (ms).
type ClockView struct {
	UsedMs int64 `json:"usedMs"`
	StepMs int64 `json:"stepMs"`
}

// ClocksData is every side's clock at the time of a packet (sides in turn order), the side to
// move (index into sides: 0 is Player1) and which sides are away (disconnected, seat kept; same
// order as sides).
type ClocksData struct {
	Sides  []ClockView `json:"sides"`
	ToMove int         `json:"toMove"`
	Away   []bool      `json:"away"`
}

// clocksAt is g's clocks at now as sent (its lock held).
func (g *game) clocksAt(now time.Time) *ClocksData {
	snap := g.clocks.snapshot(now)
	out := &ClocksData{Sides: make([]ClockView, len(snap)), ToMove: g.clocks.toMove, Away: make([]bool, len(snap))}
	for i, c := range snap {
		out.Sides[i] = ClockView{UsedMs: c.used.Milliseconds(), StepMs: c.step.Milliseconds()}
		out.Away[i] = !g.clocks.away[i].IsZero()
	}
	return out
}

// commitMove records the legal move from -> to received at now (g's lock held): the entry with the
// time the mover spent on it, taken before the clocks add it and the increment.
func (g *game) commitMove(from, to [2]int, notation string, now time.Time) {
	g.moves = append(g.moves, moveEntry{From: from, To: to, Notation: notation, Ms: g.clocks.elapsed(now).Milliseconds()})
	g.clocks.commitMove(now)
}

// otherSide is the winner (player number) when side index loses: the other player of a two-player
// game; 0 (no single winner) otherwise.
func (g *game) otherSide(side int) int {
	if len(g.players) == 2 {
		return 2 - side
	}
	return 0
}

// stopDeadline stops g's deadline timer and makes any callback already waiting a no-op (lock held).
func (g *game) stopDeadline() {
	g.gen++
	if g.deadline != nil {
		g.deadline.Stop()
		g.deadline = nil
	}
}

// armDeadline (re)arms g's deadline timer at the clocks' next expiry as of now (lock held); none
// when nothing can end the game by the clocks.
func (s *Server) armDeadline(g *game, now time.Time) {
	g.stopDeadline()
	_, _, at, ok := g.clocks.expiry(now)
	if !ok {
		return
	}
	gen := g.gen
	g.deadline = time.AfterFunc(at.Sub(now), func() { s.onDeadline(g, gen) })
}

// onDeadline is g's deadline timer firing: the game ends if its clocks have run out, otherwise the
// timer is re-armed (it fired early or the clocks changed).
func (s *Server) onDeadline(g *game, gen uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.over || g.gen != gen {
		return
	}
	now := time.Now()
	if !s.endIfExpired(g, now) {
		s.armDeadline(g, now)
	}
}

// endIfExpired ends g when its clocks have run out by now (lock held, game not over); it reports
// whether it did.
func (s *Server) endIfExpired(g *game, now time.Time) bool {
	side, reason, ok := g.clocks.expired(now)
	if !ok {
		return false
	}
	s.endGame(g, g.otherSide(side), reason)
	return true
}

// sideOf is the player number (1, 2) of c in g (lock held): by the connection, not only the
// account, so a connection replaced by a new login (see resume) no longer acts for its seat; 0 when
// c does not play in g.
func (g *game) sideOf(c *Client) int {
	for i, p := range g.players {
		if p.conn == c {
			return i + 1
		}
	}
	return 0
}

// rebind makes c the connection of accountId's player (g's lock held); false when the account does
// not play in g. For resuming after a re-login (resume).
func (g *game) rebind(accountId string, c *Client) bool {
	for i, p := range g.players {
		if accountId != "" && p.id == accountId {
			g.players[i].conn = c
			return true
		}
	}
	return false
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
	Clocks   *ClocksData     `json:"clocks"`

	// Set when a player rejoins its game after logging in again (resume): the number of moves made
	// so far and the moves themselves (Position is the current one). Absent at the game's start.
	Resumed bool        `json:"resumed,omitempty"`
	Ply     int         `json:"ply,omitempty"`
	Moves   []moveEntry `json:"moves,omitempty"`
}

// startGameData is the StartGame of g's player i (0-based) at now (lock held).
func (g *game) startGameData(i int, now time.Time) StartGameData {
	views := make([]SeatView, len(g.players))
	for j, p := range g.players {
		views[j] = SeatView{Id: p.id, Name: p.conn.Name()}
	}
	return StartGameData{
		GameId: g.id, Kind: g.kind, Rules: g.rules, Timer: g.room.Settings.Timer,
		Players: views, YourSide: i + 1, Position: g.position, Clocks: g.clocksAt(now),
	}
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
	Clocks   *ClocksData     `json:"clocks,omitempty"`
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
	s.launchGame(r, players)
}

// launchGame sets up and starts the game of r, which BeginGame marked playing with players. A
// player that left or disconnected since then makes the start fail (see RoomManager.SetGame): no
// game, the others are sent the waiting room.
func (s *Server) launchGame(r *Room, players []player) {
	// Seat 0 is the host's seat when the host created the room; "Host" / "Guest" pick by the host.
	host := s.rooms.HostOf(r)
	first := players[0]
	switch r.Settings.FirstMover {
	case "Host":
		for _, p := range players {
			if p.id == host {
				first = p
			}
		}
	case "Guest":
		for _, p := range players {
			if p.id != host {
				first = p
			}
		}
	default:
		if randomBool() {
			first = players[1]
		}
	}
	ordered := []player{first}
	for _, p := range players {
		if p.id != first.id {
			ordered = append(ordered, p)
		}
	}

	// Every room has clocks (validate requires a timer mode); a room without one would only measure
	// (count up) rather than count down from zero limits.
	timer := r.Settings.Timer
	if timer.Mode == "" {
		timer.Mode = "CountUp"
	}
	now := time.Now()
	g := &game{
		id: s.rooms.NewGameId(), room: r, kind: r.Settings.Kind, rules: r.Settings.Rules,
		players: ordered, position: standardPosition(), startedAt: now,
		clocks: newClockState(timer, len(ordered), now), done: make(chan struct{}),
	}
	// Held until the StartGame packets are out, so no action or deadline overtakes them.
	g.mu.Lock()
	defer g.mu.Unlock()
	if !s.rooms.SetGame(r, g) {
		s.sendRoomState(r)
		logger.Infof("Game start in room %s failed: a player left", r.Id)
		return
	}
	s.armDeadline(g, now)
	// From now on the players must keep sending (Heartbeat every InGameHeartbeatInterval).
	for _, p := range ordered {
		p.conn.SetInGame(true)
	}

	for i, p := range ordered {
		data, _ := json.Marshal(g.startGameData(i, now))
		p.conn.SendPacket(CreatePacket(PacketTypeStartGame, "Server", r.Id, string(data), ""))
	}
	s.sendRoomState(r)
	go s.syncTimers(g)
	logger.Infof("Game %s started in room %s", g.id, r.Id)
}

// syncTimers sends g's players their clocks (TimerSync) every TimerSyncInterval until the game ends.
func (s *Server) syncTimers(g *game) {
	ticker := time.NewTicker(TimerSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-g.done:
			return
		case <-ticker.C:
		}
		g.mu.Lock()
		if g.over {
			g.mu.Unlock()
			return
		}
		s.sendTimerSync(g, time.Now())
		g.mu.Unlock()
	}
}

// sendTimerSync sends g's players its clocks at now (lock held).
func (s *Server) sendTimerSync(g *game, now time.Time) {
	data, _ := json.Marshal(g.clocksAt(now))
	for _, p := range g.players {
		p.conn.SendPacket(CreatePacket(PacketTypeTimerSync, "Server", g.room.Id, string(data), ""))
	}
}

// setAway marks c's side of g as away (its connection ended; lock held, game not over): the seat
// is kept, the clocks keep running, the deadline is re-armed for the away side's loss and every
// player is sent the clocks (TimerSync with away set). False when c does not play in g.
func (s *Server) setAway(g *game, c *Client, now time.Time) bool {
	side := g.sideOf(c)
	if side == 0 {
		return false
	}
	g.clocks.setAway(side-1, now)
	s.armDeadline(g, now)
	s.sendTimerSync(g, now)
	logger.Infof("Game %s: player %d is away", g.id, side)
	return true
}

// resume gives c, just logged in, its account's seat back (login, loginMu held): in a running
// game the seat's player is rebound to c (seat and game), its side is back (setBack: the away
// deadline is dropped), c is sent StartGame with resumed, ply and moves and the current clocks, and
// every player a TimerSync (away cleared); in a waiting room the seat is rebound and the room's
// state sent. The seat's previous connection (dead, or live and being replaced) no longer acts for
// it: the game and the room know a player by its connection (sideOf, RoomManager.roomOf), so that
// connection's late disconnect or packets find no seat. A game that ended meanwhile (or whose
// clocks ran out: ended here first) is not resumed; its away player is out of the room already,
// a live replaced one is rebound to the waiting room.
func (s *Server) resume(c *Client) {
	id := c.Id()
	// The room / game may change between looking and locking: look again then (a game started or
	// ended meanwhile; each can happen once at most while the account is not acting).
	for attempt := 0; attempt < 4; attempt++ {
		r, g := s.rooms.SeatOf(id)
		if r == nil {
			return
		}
		if g != nil {
			if s.resumeGame(c, r, g) {
				return
			}
			continue
		}
		if done := s.rooms.RebindWaiting(r, id, c); done {
			c.SetRoom(r.Id)
			s.sendRoomState(r)
			logger.Infof("Room %s: %s rejoined", r.Id, id)
			return
		}
	}
}

// resumeGame rebinds c's account's player of g (in room r) to c and sends the resume (see resume);
// false when g is over (or ended now by its clocks) or no longer r's game, so the caller looks again.
func (s *Server) resumeGame(c *Client, r *Room, g *game) bool {
	id := c.Id()
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if g.over || s.endIfExpired(g, now) {
		return false
	}
	if !s.rooms.RebindPlaying(r, g, id, c) || !g.rebind(id, c) {
		return false
	}
	side := g.sideOf(c)
	g.clocks.setBack(side-1, now)
	s.armDeadline(g, now)
	c.SetRoom(r.Id)
	c.SetInGame(true)
	data := g.startGameData(side-1, now)
	data.Resumed, data.Ply, data.Moves = true, len(g.moves), g.moves
	b, _ := json.Marshal(data)
	c.SendPacket(CreatePacket(PacketTypeStartGame, "Server", r.Id, string(b), ""))
	view, _ := s.rooms.View(r)
	b, _ = json.Marshal(view)
	c.SendPacket(CreatePacket(PacketTypeRoomState, "Server", r.Id, string(b), ""))
	s.sendTimerSync(g, now)
	logger.Infof("Game %s: player %d is back", g.id, side)
	return true
}

// handleGameAction checks and applies a player's action in its room's game.
func (s *Server) handleGameAction(c *Client, pkt *Packet) {
	// The action is timed when received: the rules host's time is not the mover's.
	now := time.Now()
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
	if g.over || s.endIfExpired(g, now) {
		s.reject(c, act.Seq, GameErrNoGame)
		return
	}
	side := g.sideOf(c)
	if side == 0 { // c was replaced by a new login of its account meanwhile
		s.reject(c, act.Seq, GameErrNoGame)
		return
	}
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
		move := rules.Move{From: *act.From, To: *act.To}
		if res.Record.Captured != nil {
			g.history = rules.History{}
		} else {
			g.history.PliesSinceCapture++
			g.history.RecentMoves = append(g.history.RecentMoves, move)
		}
		g.commitMove(*act.From, *act.To, res.Record.Notation, now)
		if res.GameOver == nil {
			s.armDeadline(g, now)
		}
		update := GameUpdateData{Seq: act.Seq, Ply: len(g.moves), Mover: side, Record: res.Record, Position: &g.position, Check: res.Check, Clocks: g.clocksAt(now)}
		data, _ := json.Marshal(update)
		for _, p := range g.players {
			p.conn.SendPacket(CreatePacket(PacketTypeGameUpdate, "Server", g.room.Id, string(data), ""))
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
// waiting (everyone's ready cleared) and the away players (connection gone) out of it. winner is a
// player number, 0 for a draw.
func (s *Server) endGame(g *game, winner int, reason string) {
	g.over = true
	close(g.done)
	g.stopDeadline()
	data, _ := json.Marshal(EndGameData{Winner: winner, Reason: reason})
	for _, p := range g.players {
		p.conn.SetInGame(false)
		p.conn.SendPacket(CreatePacket(PacketTypeEndGame, "Server", g.room.Id, string(data), ""))
	}

	ids := make([]string, len(g.players))
	for i, p := range g.players {
		ids[i] = p.id
	}
	record := &db.Game{
		ID: g.id, Kind: g.kind, Mode: g.room.Settings.Mode, Players: mustJSON(ids), Reason: reason,
		Moves: mustJSON(g.moves), StartedAt: g.startedAt, EndedAt: time.Now(),
	}
	record.Clocks = mustJSON(g.clocksAt(record.EndedAt))
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
	// An away player's seat holds a dead connection: it leaves now (the next player becomes the
	// host if it was; the room goes when empty).
	for i, p := range g.players {
		if !g.clocks.away[i].IsZero() {
			if r, _ := s.rooms.Leave(p.conn); r != nil {
				p.conn.SetRoom("")
			}
		}
	}
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
