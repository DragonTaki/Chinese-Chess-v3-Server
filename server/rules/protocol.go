/* ----- ----- ----- ----- */
// protocol.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/06
// Update Date: 2026/10/06
// Version: v1.0
/* ----- ----- ----- ----- */

package rules

// The rules host protocol's messages (docs/RULES-HOST.md in the record repo). Enum values are
// the C# names: kinds Traditional / DarkHalf / OpenHalf, types General / Advisor / Elephant /
// Chariot / Horse / Cannon / Soldier, colours Red / Black.

// Piece is one piece of a position. Side is the owning player (1 / 2), 0 while a dark-chess
// game's factions are undecided. A face-down piece carries its real identity: positions only
// travel between the server and the rules host, never to a client.
type Piece struct {
	Type   string `json:"type"`
	Color  string `json:"color"`
	Side   int    `json:"side"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	FaceUp bool   `json:"faceUp"`
}

// Position is whose turn it is (1 / 2) and every piece on the board.
type Position struct {
	ToMove int     `json:"toMove"`
	Pieces []Piece `json:"pieces"`
}

// Move is a move by squares ([x, y]).
type Move struct {
	From [2]int `json:"from"`
	To   [2]int `json:"to"`
}

// History is what the repetition / no-progress rules need: plies since the last capture (or
// dark-chess flip) and the moves since then. Kept by the server; not used by the host yet.
type History struct {
	PliesSinceCapture int    `json:"pliesSinceCapture"`
	RecentMoves       []Move `json:"recentMoves"`
}

// Action is what a validate request asks for: Type "move" (From, To) or "flip" (At).
type Action struct {
	Type string  `json:"type"`
	From *[2]int `json:"from,omitempty"`
	To   *[2]int `json:"to,omitempty"`
	At   *[2]int `json:"at,omitempty"`
}

// MoveAction is a move action.
func MoveAction(fromX, fromY, toX, toY int) Action {
	return Action{Type: "move", From: &[2]int{fromX, fromY}, To: &[2]int{toX, toY}}
}

// FlipAction is a flip action.
func FlipAction(x, y int) Action {
	return Action{Type: "flip", At: &[2]int{x, y}}
}

// Game is what every request about a game carries: the kind, the game's rule switches (the
// names of the client's Rules, e.g. "canCaptureHiddenPiece"; nil for the defaults) and the position.
type Game struct {
	Kind     string          `json:"kind"`
	Rules    map[string]bool `json:"rules,omitempty"`
	Position Position        `json:"position"`
}

// Record is a validated move's record.
type Record struct {
	Kind     string `json:"kind"`
	Notation string `json:"notation"`
	Captured *Piece `json:"captured"`
	Revealed *Piece `json:"revealed"`
}

// GameOver is how a game ended: the winner (1 / 2; 0 for a draw) and why (GameOverReason name).
type GameOver struct {
	Winner int    `json:"winner"`
	Reason string `json:"reason"`
}

// ValidateResult is the answer to Validate. When Legal is false only Reason is set
// (NoPieceThere, FaceDownPiece, NotYourPiece, IllegalMove, CannotFlip, AlreadyFaceUp).
type ValidateResult struct {
	Legal           bool      `json:"legal"`
	Reason          string    `json:"reason"`
	Position        *Position `json:"position"`
	Record          *Record   `json:"record"`
	FactionsDecided bool      `json:"factionsDecided"`
	Check           bool      `json:"check"`
	GameOver        *GameOver `json:"gameOver"`
}

// LegalMovesResult is the answer to LegalMoves: the legal moves and the face-down pieces that may be flipped.
type LegalMovesResult struct {
	Moves []Move   `json:"moves"`
	Flips [][2]int `json:"flips"`
}
