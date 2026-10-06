/* ----- ----- ----- ----- */
// protocol.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/06
// Update Date: 2026/10/06
// Version: v1.1
/* ----- ----- ----- ----- */

package rules

// The rules host protocol's messages (docs/RULES-HOST.md in the record repo). Enum values are
// the C# names: kinds Traditional / Flip / DarkHalf / OpenHalf / ThreeKingdoms, types General /
// Advisor / Elephant / Chariot / Horse / Cannon / Soldier, colours Red / Black.

// Piece is one piece of a position. Side is the owning player (1 / 2; Three Kingdoms 1..3), 0
// while a dark-chess game's factions (a Three Kingdoms team) are undecided. A face-down piece carries its real identity: positions only
// travel between the server and the rules host, never to a client.
type Piece struct {
	Type   string `json:"type"`
	Color  string `json:"color"`
	Side   int    `json:"side"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	FaceUp bool   `json:"faceUp"`
}

// Position is whose turn it is (1 / 2; Three Kingdoms 1..3), every piece on the board and, for
// Three Kingdoms only, the game's state beyond the pieces.
type Position struct {
	ToMove        int            `json:"toMove"`
	Pieces        []Piece        `json:"pieces"`
	ThreeKingdoms *ThreeKingdoms `json:"threeKingdoms,omitempty"`
}

// TeamSplit is a Three Kingdoms custom team split: colour ("Red", "Black") -> piece type ->
// team (1..3). Every team needs at least one piece; a team's threshold is its piece count.
type TeamSplit map[string]map[string]int

// ThreeKingdoms is a Three Kingdoms game's state, each array by player (Player1 first): the team
// each claimed (0 = none yet, 1..3), the points, whether it resigned, whether its clock ran out
// (it left), the order it went out in (0 = still in, 1 = first out...) and when it last scored
// (0 = never, 1 = the game's first score...; equal scores rank whoever was first ahead).
type ThreeKingdoms struct {
	Teams      [3]int  `json:"teams"`
	Scores     [3]int  `json:"scores"`
	Resigned   [3]bool `json:"resigned"`
	TimedOut   [3]bool `json:"timedOut"`
	OutOrder   [3]int  `json:"outOrder"`
	ScoreOrder [3]int  `json:"scoreOrder"`
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

// Game is what every request about a game carries: the kind, the game's rules (the names of
// the client's Rules: switches such as "canCaptureHiddenPiece": true, and Three Kingdoms'
// "halfCrossWinCondition": "Points" and "halfCrossTeams" (colour -> type -> team, TeamSplit);
// nil for the defaults) and the position.
type Game struct {
	Kind     string         `json:"kind"`
	Rules    map[string]any `json:"rules,omitempty"`
	Position Position       `json:"position"`
}

// Record is a validated move's record.
type Record struct {
	Kind     string `json:"kind"`
	Notation string `json:"notation"`
	Captured *Piece `json:"captured"`
	Revealed *Piece `json:"revealed"`
}

// GameOver is how a game ended: the winner (1 / 2; Three Kingdoms 1..3; 0 for a draw), why
// (GameOverReason name) and, Three Kingdoms only, every player's place (first first).
type GameOver struct {
	Winner  int    `json:"winner"`
	Reason  string `json:"reason"`
	Ranking []int  `json:"ranking,omitempty"`
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
