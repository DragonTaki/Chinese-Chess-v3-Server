/* ----- ----- ----- ----- */
// room.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// Online play (ONLINE-PLAY.md in the record repo): a player picks the game kind, then the mode.
// Only the Traditional kind and the custom room (自訂房間) mode are open so far; only a custom room
// chooses its rules, clocks, first mover and whether pausing is allowed (author 2026-10-07).

// Room error codes, sent as an Error packet's data.
const (
	RoomErrInvalidData     = "InvalidData"
	RoomErrKindNotOpen     = "KindNotOpen"
	RoomErrModeNotOpen     = "ModeNotOpen"
	RoomErrInvalidSettings = "InvalidSettings"
	RoomErrAlreadyInRoom   = "AlreadyInRoom"
	RoomErrNotInRoom       = "NotInRoom"
	RoomErrRoomNotFound    = "RoomNotFound"
	RoomErrRoomFull        = "RoomFull"
	RoomErrRoomNotWaiting  = "RoomNotWaiting"
)

// Room states.
const (
	RoomWaiting = "Waiting"
	RoomPlaying = "Playing"
)

// Timer limits, the same as the client's rule settings (RuleSettings in Game/Configs).
const (
	TotalMinutesMin, TotalMinutesMax         = 1, 600
	StepSecondsMin, StepSecondsMax           = 1, 3600
	IncrementSecondsMin, IncrementSecondsMax = 0, 600
)

// openRules are the rule switches a custom room of each open kind may set (the rules host's names:
// the client's Rules properties, camelCase).
var openRules = map[string][]string{
	"Traditional": {
		"canGeneralSeeGeneral", "canGeneralLeavePalace", "canAdvisorLeavePalace",
		"canElephantEyeBlocked", "canHorseLegHobbled", "canCaptureOwnPiece",
	},
}

// seatCount is the number of players of each open kind.
var seatCount = map[string]int{"Traditional": 2}

// TimerSettings are a room's clocks: count down (局時, 步時, 加秒) or count up (only measuring).
type TimerSettings struct {
	Mode             string `json:"mode"` // "CountDown" or "CountUp"
	TotalMinutes     int    `json:"totalMinutes"`
	StepSeconds      int    `json:"stepSeconds"`
	StepTimer        bool   `json:"stepTimer"`
	IncrementSeconds int    `json:"incrementSeconds"`
}

// RoomSettings are what a room is created with: the game kind, the mode and, for a custom room,
// the rule switches (nil: the defaults), the clocks, the first mover ("Host", "Guest" or "Random")
// and whether pausing is allowed.
type RoomSettings struct {
	Kind       string          `json:"kind"`
	Mode       string          `json:"mode"`
	Rules      map[string]bool `json:"rules,omitempty"`
	Timer      TimerSettings   `json:"timer"`
	FirstMover string          `json:"firstMover"`
	AllowPause bool            `json:"allowPause"`
}

// validate checks the settings of a new room; it returns the room error code and what is wrong.
func (rs *RoomSettings) validate() (string, string) {
	allowed, open := openRules[rs.Kind]
	if !open {
		return RoomErrKindNotOpen, rs.Kind
	}
	if rs.Mode != "Custom" {
		return RoomErrModeNotOpen, rs.Mode
	}
	for name := range rs.Rules {
		if !contains(allowed, name) {
			return RoomErrInvalidSettings, "unknown rule " + name
		}
	}
	t := rs.Timer
	if t.Mode != "CountDown" && t.Mode != "CountUp" {
		return RoomErrInvalidSettings, "timer mode " + t.Mode
	}
	if t.TotalMinutes < TotalMinutesMin || t.TotalMinutes > TotalMinutesMax {
		return RoomErrInvalidSettings, fmt.Sprintf("total minutes %d", t.TotalMinutes)
	}
	if t.StepSeconds < StepSecondsMin || t.StepSeconds > StepSecondsMax {
		return RoomErrInvalidSettings, fmt.Sprintf("step seconds %d", t.StepSeconds)
	}
	if t.IncrementSeconds < IncrementSecondsMin || t.IncrementSeconds > IncrementSecondsMax {
		return RoomErrInvalidSettings, fmt.Sprintf("increment seconds %d", t.IncrementSeconds)
	}
	if rs.FirstMover != "Host" && rs.FirstMover != "Guest" && rs.FirstMover != "Random" {
		return RoomErrInvalidSettings, "first mover " + rs.FirstMover
	}
	return "", ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// seat is one place at a room's table; client is nil while it is free.
type seat struct {
	client *Client
	ready  bool
}

// Room is a table players sit at: its settings, its host and its seats. Guarded by the
// RoomManager's lock.
type Room struct {
	Id       string
	Settings RoomSettings
	Host     *Client
	State    string
	seats    []seat
	game     *game // the game in progress; nil while waiting
}

// SeatView is one seat as the clients see it.
type SeatView struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// RoomView is a room as the clients see it (RoomState; one entry of RoomList). A free seat is null.
type RoomView struct {
	RoomId   string       `json:"roomId"`
	Settings RoomSettings `json:"settings"`
	HostId   string       `json:"hostId"`
	Seats    []*SeatView  `json:"seats"`
	State    string       `json:"state"`
}

func (r *Room) view() RoomView {
	v := RoomView{RoomId: r.Id, Settings: r.Settings, State: r.State, Seats: make([]*SeatView, len(r.seats))}
	if r.Host != nil {
		v.HostId = r.Host.Id()
	}
	for i, s := range r.seats {
		if s.client != nil {
			v.Seats[i] = &SeatView{Id: s.client.Id(), Name: s.client.Name(), Ready: s.ready}
		}
	}
	return v
}

func (r *Room) members() []*Client {
	var list []*Client
	for _, s := range r.seats {
		if s.client != nil {
			list = append(list, s.client)
		}
	}
	return list
}

func (r *Room) seatOf(c *Client) int {
	for i, s := range r.seats {
		if s.client == c {
			return i
		}
	}
	return -1
}

// RoomManager keeps every room; one lock for all of them (rooms change rarely).
type RoomManager struct {
	mu     sync.Mutex
	rooms  map[string]*Room
	byUser map[*Client]*Room
}

// NewRoomManager makes an empty room manager.
func NewRoomManager() *RoomManager {
	return &RoomManager{rooms: map[string]*Room{}, byUser: map[*Client]*Room{}}
}

// newRoomId returns an unused six-digit room number (房號), drawn with crypto/rand.
func (m *RoomManager) newRoomId() string {
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(900000))
		if err != nil {
			panic(err) // crypto/rand does not fail on supported platforms
		}
		id := fmt.Sprintf("%06d", n.Int64()+100000)
		if _, used := m.rooms[id]; !used {
			return id
		}
	}
}

// Create makes a room with settings; c becomes its host, in the first seat.
func (m *RoomManager) Create(c *Client, settings RoomSettings) (*Room, string, string) {
	if code, detail := settings.validate(); code != "" {
		return nil, code, detail
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, in := m.byUser[c]; in {
		return nil, RoomErrAlreadyInRoom, ""
	}
	r := &Room{Id: m.newRoomId(), Settings: settings, Host: c, State: RoomWaiting, seats: make([]seat, seatCount[settings.Kind])}
	r.seats[0].client = c
	m.rooms[r.Id] = r
	m.byUser[c] = r
	return r, "", ""
}

// Join seats c in the first free seat of the room roomId; everyone's ready is cleared.
func (m *RoomManager) Join(c *Client, roomId string) (*Room, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, in := m.byUser[c]; in {
		return nil, RoomErrAlreadyInRoom
	}
	r, ok := m.rooms[roomId]
	if !ok {
		return nil, RoomErrRoomNotFound
	}
	if r.State != RoomWaiting {
		return nil, RoomErrRoomNotWaiting
	}
	free := r.seatOf(nil)
	if free < 0 {
		return nil, RoomErrRoomFull
	}
	r.seats[free].client = c
	for i := range r.seats {
		r.seats[i].ready = false
	}
	m.byUser[c] = r
	return r, ""
}

// Leave takes c out of its room (a disconnect too): the room is removed when empty, otherwise the
// next seated player becomes the host if c was it, and everyone's ready is cleared. It returns the
// room left (nil when c was in none) and the players still in it.
func (m *RoomManager) Leave(c *Client) (*Room, []*Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.byUser[c]
	if !ok {
		return nil, nil
	}
	delete(m.byUser, c)
	r.seats[r.seatOf(c)] = seat{}
	for i := range r.seats {
		r.seats[i].ready = false
	}
	rest := r.members()
	if len(rest) == 0 {
		delete(m.rooms, r.Id)
	} else if r.Host == c {
		r.Host = rest[0]
	}
	return r, rest
}

// SetReady sets c's ready mark; it returns the room, whether every seat is taken and ready, and an error code.
func (m *RoomManager) SetReady(c *Client, ready bool) (*Room, bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.byUser[c]
	if !ok {
		return nil, false, RoomErrNotInRoom
	}
	if r.State != RoomWaiting {
		return nil, false, RoomErrRoomNotWaiting
	}
	r.seats[r.seatOf(c)].ready = ready
	all := true
	for _, s := range r.seats {
		if s.client == nil || !s.ready {
			all = false
		}
	}
	return r, all, ""
}

// Members is everyone seated in r.
func (m *RoomManager) Members(r *Room) []*Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return r.members()
}

// HostOf is r's host.
func (m *RoomManager) HostOf(r *Room) *Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return r.Host
}

// BeginGame marks r playing when every seat is taken and ready; it returns the players in seat
// order and false when the room cannot start (someone left or unreadied meanwhile, or it already started).
func (m *RoomManager) BeginGame(r *Room) ([]*Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.State != RoomWaiting {
		return nil, false
	}
	for _, s := range r.seats {
		if s.client == nil || !s.ready {
			return nil, false
		}
	}
	r.State = RoomPlaying
	return r.members(), true
}

// SetGame records r's game in progress.
func (m *RoomManager) SetGame(r *Room, g *game) {
	m.mu.Lock()
	r.game = g
	m.mu.Unlock()
}

// GameOf is the game in progress in c's room; nil when there is none.
func (m *RoomManager) GameOf(c *Client) *game {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.byUser[c]; ok {
		return r.game
	}
	return nil
}

// EndGame puts r back to waiting: no game, everyone's ready cleared.
func (m *RoomManager) EndGame(r *Room) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.State = RoomWaiting
	r.game = nil
	for i := range r.seats {
		r.seats[i].ready = false
	}
}

// NewGameId returns a new game id (16 random hex digits from crypto/rand).
func (m *RoomManager) NewGameId() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", b)
}

// View is the room's current state for the clients (taken under the lock).
func (m *RoomManager) View(r *Room) (RoomView, []*Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return r.view(), r.members()
}

// List is every waiting room (房間列表).
func (m *RoomManager) List() []RoomView {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := []RoomView{}
	for _, r := range m.rooms {
		if r.State == RoomWaiting {
			list = append(list, r.view())
		}
	}
	return list
}

// ----- Packet handling -----

// sendRoomError sends c a room error code (with the detail, when there is one).
func sendRoomError(c *Client, code, detail string) {
	data := code
	if detail != "" {
		data = code + ": " + detail
	}
	c.SendPacket(CreatePacket(PacketTypeError, "Server", "", data, ""))
}

// sendRoomState sends every member of r its current state.
func (s *Server) sendRoomState(r *Room) {
	view, members := s.rooms.View(r)
	data, _ := json.Marshal(view)
	for _, m := range members {
		m.SendPacket(CreatePacket(PacketTypeRoomState, "Server", r.Id, string(data), ""))
	}
}

// handleRoomPacket handles the room packets; it returns false for any other packet type.
func (s *Server) handleRoomPacket(c *Client, pkt *Packet) bool {
	switch pkt.Type {
	case PacketTypeRoomList:
		data, _ := json.Marshal(s.rooms.List())
		c.SendPacket(CreatePacket(PacketTypeRoomList, "Server", "", string(data), ""))
	case PacketTypeCreateRoom:
		var settings RoomSettings
		dec := json.NewDecoder(strings.NewReader(pkt.Data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&settings); err != nil {
			sendRoomError(c, RoomErrInvalidData, err.Error())
			return true
		}
		r, code, detail := s.rooms.Create(c, settings)
		if code != "" {
			sendRoomError(c, code, detail)
			return true
		}
		c.SetRoom(r.Id)
		s.sendRoomState(r)
	case PacketTypeJoinRoom:
		var req struct {
			RoomId string `json:"roomId"`
		}
		if err := json.Unmarshal([]byte(pkt.Data), &req); err != nil {
			sendRoomError(c, RoomErrInvalidData, err.Error())
			return true
		}
		r, code := s.rooms.Join(c, req.RoomId)
		if code != "" {
			sendRoomError(c, code, "")
			return true
		}
		c.SetRoom(r.Id)
		s.sendRoomState(r)
	case PacketTypeLeaveRoom:
		s.leaveRoom(c, true)
	case PacketTypeReady:
		var req struct {
			Ready bool `json:"ready"`
		}
		if err := json.Unmarshal([]byte(pkt.Data), &req); err != nil {
			sendRoomError(c, RoomErrInvalidData, err.Error())
			return true
		}
		r, all, code := s.rooms.SetReady(c, req.Ready)
		if code != "" {
			sendRoomError(c, code, "")
			return true
		}
		s.sendRoomState(r)
		if all {
			s.startGame(r)
		}
	default:
		return false
	}
	return true
}

// leaveRoom takes c out of its room and tells the others; reportNotInRoom answers a LeaveRoom
// sent outside any room (a disconnect says nothing). Leaving during a game resigns it first (for
// now a disconnect too: keeping the seat while the clock runs comes later).
func (s *Server) leaveRoom(c *Client, reportNotInRoom bool) {
	if g := s.rooms.GameOf(c); g != nil {
		g.mu.Lock()
		// A clock already run out ends the game by it, not by the resign.
		if !g.over && !s.endIfExpired(g, time.Now()) {
			if side := g.sideOf(c); side > 0 {
				s.endGame(g, 3-side, "Resign")
			}
		}
		g.mu.Unlock()
	}
	r, rest := s.rooms.Leave(c)
	if r == nil {
		if reportNotInRoom {
			sendRoomError(c, RoomErrNotInRoom, "")
		}
		return
	}
	c.SetRoom("")
	if len(rest) > 0 {
		s.sendRoomState(r)
	}
}
