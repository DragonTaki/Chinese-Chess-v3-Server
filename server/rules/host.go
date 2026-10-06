/* ----- ----- ----- ----- */
// host.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/06
// Update Date: 2026/10/06
// Version: v1.0
/* ----- ----- ----- ----- */

// Package rules runs the C# rules host (the shared rules repo's RulesHost) as a child process and
// asks it whether moves are legal: one process for the whole server, one JSON request per line
// on its standard input, answers matched back to the waiting goroutine by id (the host answers
// requests in parallel, so in any order). The process is watched: when it dies it is started
// again and the requests it had not answered are sent once more.
package rules

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"Chinese-Chess-v3-Server/logger"
)

// Errors returned by Host calls.
var (
	ErrClosed     = errors.New("rules host closed")
	ErrRestarted  = errors.New("rules host restarted before answering")
	ErrBadRequest = errors.New("rules host rejected the request")
)

const (
	// DefaultTimeout bounds one request (a request normally takes well under a millisecond).
	DefaultTimeout = 5 * time.Second
	// pingInterval is how often an idle host is checked.
	pingInterval = 10 * time.Second
	// restartDelay keeps a host that dies at once from being restarted in a tight loop.
	restartDelay = time.Second
	// maxLine is the longest answer line accepted.
	maxLine = 4 << 20
)

// answer is one response line, or the reason none will come.
type answer struct {
	raw []byte
	err error
}

// Host is the running rules host. Safe for concurrent use.
type Host struct {
	path    string
	timeout time.Duration

	// writeMu serializes writes to the process's standard input (one whole line at a time). It is
	// separate from mu, so a write that blocks never keeps answers from being delivered.
	writeMu sync.Mutex

	mu      sync.Mutex // guards everything below
	stdin   io.WriteCloser
	cmd     *exec.Cmd
	exited  chan struct{} // closed when the current process has ended
	pending map[string]chan answer
	nextID  uint64
	closed  bool
	ready   chan struct{} // closed once a process is running; replaced on every restart
}

// Start starts the rules host at path (the built ChineseChess.RulesHost.dll, run with dotnet,
// or a native executable) and watches it until Close.
func Start(path string) (*Host, error) {
	h := &Host{path: path, timeout: DefaultTimeout, pending: make(map[string]chan answer)}
	if err := h.launch(); err != nil {
		return nil, err
	}
	go h.pingLoop()
	return h, nil
}

// launch starts a process and its reader and watcher goroutines. Called with mu not held.
func (h *Host) launch() error {
	var cmd *exec.Cmd
	if strings.HasSuffix(strings.ToLower(h.path), ".dll") {
		cmd = exec.Command("dotnet", h.path)
	} else {
		cmd = exec.Command(h.path)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start rules host %s: %w", h.path, err)
	}

	exited := make(chan struct{})
	h.mu.Lock()
	h.cmd = cmd
	h.stdin = stdin
	h.exited = exited
	if h.ready == nil {
		h.ready = make(chan struct{})
	}
	close(h.ready)
	h.mu.Unlock()

	logger.Infof("Rules host started: %s (pid %d)", h.path, cmd.Process.Pid)
	go h.readAnswers(stdout)
	go h.forwardLog(stderr)
	go h.watch(cmd, exited)
	return nil
}

// readAnswers hands every answer line to the request waiting for its id.
func (h *Host) readAnswers(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var head struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(line, &head); err != nil || head.ID == "" {
			logger.Warnf("Rules host: unreadable answer: %s", string(line))
			continue
		}
		h.mu.Lock()
		ch, ok := h.pending[head.ID]
		delete(h.pending, head.ID)
		h.mu.Unlock()
		if ok {
			ch <- answer{raw: line}
		}
	}
}

// forwardLog writes the host's log lines (its standard error) to the server log.
func (h *Host) forwardLog(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		logger.Warnf("Rules host: %s", scanner.Text())
	}
}

// watch waits for the process to end; unless the host is closed, it fails the unanswered
// requests with ErrRestarted (their callers send them again) and starts a new process.
func (h *Host) watch(cmd *exec.Cmd, exited chan struct{}) {
	err := cmd.Wait()

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(exited)
		return
	}
	logger.Warnf("Rules host exited (%v); restarting", err)
	h.ready = make(chan struct{}) // new requests wait for the next process
	failed := h.pending
	h.pending = make(map[string]chan answer)
	h.mu.Unlock()
	// Closed after ready is replaced: a caller that saw this process fail and waits for it to
	// end then finds the new ready channel, not the old one.
	close(exited)

	for _, ch := range failed {
		ch <- answer{err: ErrRestarted}
	}

	for {
		time.Sleep(restartDelay)
		h.mu.Lock()
		closed := h.closed
		h.mu.Unlock()
		if closed {
			return
		}
		if err := h.launch(); err != nil {
			logger.Errorf("Rules host restart failed: %v", err)
			continue
		}
		return
	}
}

// pingLoop checks the host every pingInterval; a host that does not answer is killed (and so restarted).
func (h *Host) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.Lock()
		closed, cmd := h.closed, h.cmd
		h.mu.Unlock()
		if closed {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
		err := h.Ping(ctx)
		cancel()
		if err != nil && !errors.Is(err, ErrClosed) && cmd != nil && cmd.Process != nil {
			logger.Warnf("Rules host did not answer a ping (%v); killing it", err)
			_ = cmd.Process.Kill()
		}
	}
}

// call sends one request (op plus the fields of body) and returns the answer line. A request
// whose process died before answering is sent once more to the restarted process.
func (h *Host) call(ctx context.Context, op string, body any) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.timeout)
		defer cancel()
	}

	for attempt := 0; ; attempt++ {
		raw, err := h.callOnce(ctx, op, body)
		if errors.Is(err, ErrRestarted) && attempt == 0 {
			continue
		}
		return raw, err
	}
}

func (h *Host) callOnce(ctx context.Context, op string, body any) ([]byte, error) {
	// Wait for a running process (only waits while one is being restarted).
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, ErrClosed
	}
	ready := h.ready
	h.mu.Unlock()
	select {
	case <-ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	line, id, err := h.encode(op, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan answer, 1)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, ErrClosed
	}
	h.pending[id] = ch
	stdin, exited := h.stdin, h.exited
	h.mu.Unlock()

	h.writeMu.Lock()
	_, err = stdin.Write(line)
	h.writeMu.Unlock()
	if err != nil {
		// The process is dying: wait until it is gone (and a restart is under way) before the
		// caller sends the request again.
		h.forget(id)
		select {
		case <-exited:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, ErrRestarted
	}

	select {
	case a := <-ch:
		return a.raw, a.err
	case <-ctx.Done():
		h.forget(id)
		return nil, ctx.Err()
	}
}

// encode builds the request line: {"id": ..., "op": ..., <body's fields>}.
func (h *Host) encode(op string, body any) ([]byte, string, error) {
	fields := map[string]any{}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(b, &fields); err != nil {
			return nil, "", err
		}
	}
	h.mu.Lock()
	h.nextID++
	id := strconv.FormatUint(h.nextID, 10)
	h.mu.Unlock()
	fields["id"] = id
	fields["op"] = op
	line, err := json.Marshal(fields)
	if err != nil {
		return nil, "", err
	}
	return append(line, '\n'), id, nil
}

func (h *Host) forget(id string) {
	h.mu.Lock()
	delete(h.pending, id)
	h.mu.Unlock()
}

// decode reads an answer: ErrBadRequest (with the host's reason) when ok is false, else the
// fields into result (if not nil).
func decode(raw []byte, result any) error {
	var head struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return fmt.Errorf("unreadable rules host answer: %w", err)
	}
	if !head.OK {
		return fmt.Errorf("%w: %s", ErrBadRequest, head.Error)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(raw, result)
}

// Ping checks that the host answers.
func (h *Host) Ping(ctx context.Context) error {
	raw, err := h.call(ctx, "ping", nil)
	if err != nil {
		return err
	}
	return decode(raw, nil)
}

// Validate checks action in game (with history for the repetition / no-progress rules; may be
// nil) and, when legal, returns the position after it, its record, whether it gave check and
// whether the game is over. An illegal action is not an error: Legal is false and Reason says why.
func (h *Host) Validate(ctx context.Context, game Game, action Action, history *History) (*ValidateResult, error) {
	body := struct {
		Game
		Action  Action   `json:"action"`
		History *History `json:"history,omitempty"`
	}{game, action, history}
	raw, err := h.call(ctx, "validate", body)
	if err != nil {
		return nil, err
	}
	var result ValidateResult
	if err := decode(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// LegalMoves lists the side to move's legal moves and the pieces it may flip; with from set,
// only those of the piece on that square.
func (h *Host) LegalMoves(ctx context.Context, game Game, from *[2]int) (*LegalMovesResult, error) {
	body := struct {
		Game
		From *[2]int `json:"from,omitempty"`
	}{game, from}
	raw, err := h.call(ctx, "legalMoves", body)
	if err != nil {
		return nil, err
	}
	var result LegalMovesResult
	if err := decode(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Close stops the host (its standard input is closed, so it exits after its last answers) and
// fails any request still waiting with ErrClosed.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	stdin, cmd, exited := h.stdin, h.cmd, h.exited
	waiting := h.pending
	h.pending = make(map[string]chan answer)
	h.mu.Unlock()

	for _, ch := range waiting {
		ch <- answer{err: ErrClosed}
	}
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil && exited != nil {
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	}
	return nil
}
