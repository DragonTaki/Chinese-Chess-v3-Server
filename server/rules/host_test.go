/* ----- ----- ----- ----- */
// host_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/06
// Update Date: 2026/10/06
// Version: v1.0
/* ----- ----- ----- ----- */

package rules

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// hostPath is the rules host the tests run: CHESS_RULES_PATH, or built from the rules submodule.
var hostPath string

func TestMain(m *testing.M) {
	hostPath = os.Getenv("CHESS_RULES_PATH")
	if hostPath == "" {
		out, err := os.MkdirTemp("", "rules-host-test")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer os.RemoveAll(out)
		project := filepath.Join("..", "..", "rules", "RulesHost", "ChineseChess.RulesHost.csproj")
		build := exec.Command("dotnet", "build", project, "-c", "Release", "-o", out)
		if b, err := build.CombinedOutput(); err != nil {
			fmt.Printf("building the rules host failed: %v\n%s\n", err, b)
			os.Exit(1)
		}
		hostPath = filepath.Join(out, "ChineseChess.RulesHost.dll")
	}
	os.Exit(m.Run())
}

func startHost(t *testing.T) *Host {
	t.Helper()
	h, err := Start(hostPath)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// startPosition is the standard start, red (player 1) to move.
func startPosition() Position {
	type p = Piece
	red := func(t string, x, y int) p { return p{Type: t, Color: "Red", Side: 1, X: x, Y: y, FaceUp: true} }
	black := func(t string, x, y int) p { return p{Type: t, Color: "Black", Side: 2, X: x, Y: y, FaceUp: true} }
	back := []string{"Chariot", "Horse", "Elephant", "Advisor", "General", "Advisor", "Elephant", "Horse", "Chariot"}
	var pieces []Piece
	for x, t := range back {
		pieces = append(pieces, black(t, x, 0), red(t, x, 9))
	}
	for _, x := range []int{1, 7} {
		pieces = append(pieces, black("Cannon", x, 2), red("Cannon", x, 7))
	}
	for _, x := range []int{0, 2, 4, 6, 8} {
		pieces = append(pieces, black("Soldier", x, 3), red("Soldier", x, 6))
	}
	return Position{ToMove: 1, Pieces: pieces}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestPingAndValidate(t *testing.T) {
	h := startHost(t)
	if err := h.Ping(ctx(t)); err != nil {
		t.Fatalf("ping: %v", err)
	}
	game := Game{Kind: "Traditional", Position: startPosition()}

	r, err := h.Validate(ctx(t), game, MoveAction(7, 7, 4, 7), nil)
	if err != nil || !r.Legal || r.Record.Notation != "炮二平五" || r.Position.ToMove != 2 || r.GameOver != nil {
		t.Fatalf("炮二平五: %+v, %v", r, err)
	}
	r, err = h.Validate(ctx(t), game, MoveAction(7, 7, 6, 6), nil)
	if err != nil || r.Legal || r.Reason != "IllegalMove" {
		t.Fatalf("illegal move: %+v, %v", r, err)
	}
	moves, err := h.LegalMoves(ctx(t), game, nil)
	if err != nil || len(moves.Moves) != 44 {
		t.Fatalf("legal moves: %d, %v", len(moves.Moves), err)
	}
}

func TestCheckmate(t *testing.T) {
	h := startHost(t)
	game := Game{Kind: "Traditional", Position: Position{ToMove: 1, Pieces: []Piece{
		{Type: "General", Color: "Red", Side: 1, X: 3, Y: 9, FaceUp: true},
		{Type: "Chariot", Color: "Red", Side: 1, X: 0, Y: 1, FaceUp: true},
		{Type: "Chariot", Color: "Red", Side: 1, X: 8, Y: 9, FaceUp: true},
		{Type: "General", Color: "Black", Side: 2, X: 4, Y: 0, FaceUp: true},
	}}}
	r, err := h.Validate(ctx(t), game, MoveAction(8, 9, 8, 0), nil)
	if err != nil || !r.Legal || !r.Check || r.GameOver == nil || r.GameOver.Winner != 1 || r.GameOver.Reason != "Checkmate" {
		t.Fatalf("checkmate: %+v, %v", r, err)
	}
}

func TestDarkChessFlip(t *testing.T) {
	h := startHost(t)
	game := Game{Kind: "DarkHalf", Position: Position{ToMove: 1, Pieces: []Piece{
		{Type: "Chariot", Color: "Red", X: 0, Y: 0},
		{Type: "Soldier", Color: "Black", X: 1, Y: 0},
		{Type: "General", Color: "Black", X: 7, Y: 3},
		{Type: "General", Color: "Red", X: 6, Y: 3},
	}}}
	r, err := h.Validate(ctx(t), game, FlipAction(0, 0), nil)
	if err != nil || !r.Legal || !r.FactionsDecided || r.Record.Kind != "Flip" || r.Record.Revealed.Type != "Chariot" {
		t.Fatalf("flip: %+v, %v", r, err)
	}
}

func TestBadRequest(t *testing.T) {
	h := startHost(t)
	_, err := h.Validate(ctx(t), Game{Kind: "ThreeKingdoms", Position: startPosition()}, MoveAction(0, 0, 0, 1), nil)
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("unsupported kind: want ErrBadRequest, got %v", err)
	}
}

// Many goroutines at once: every answer reaches its own caller.
func TestConcurrent(t *testing.T) {
	h := startHost(t)
	game := Game{Kind: "Traditional", Position: startPosition()}
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Even: a legal cannon move; odd: an illegal one.
			action := MoveAction(7, 7, 4, 7)
			if i%2 == 1 {
				action = MoveAction(7, 7, 6, 6)
			}
			r, err := h.Validate(ctx(t), game, action, nil)
			if err != nil {
				errs <- err
				return
			}
			if r.Legal != (i%2 == 0) {
				errs <- fmt.Errorf("request %d: legal = %v", i, r.Legal)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// A host that dies is started again and the next requests are answered.
func TestRestart(t *testing.T) {
	h := startHost(t)
	h.mu.Lock()
	pid := h.cmd.Process.Pid
	_ = h.cmd.Process.Kill()
	h.mu.Unlock()

	if err := h.Ping(ctx(t)); err != nil {
		t.Fatalf("ping after kill: %v", err)
	}
	h.mu.Lock()
	newPid := h.cmd.Process.Pid
	h.mu.Unlock()
	if newPid == pid {
		t.Fatalf("expected a new process")
	}
}
