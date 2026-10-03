package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// errMPVNotRunning is returned when there is no live mpv IPC socket.
var errMPVNotRunning = errors.New("mpv is not running")

// Player is the single owner of the mpv instance ytplay drives. Every IPC
// command, the socket path, the liveness probe and process spawning live here,
// so the rest of the app never touches the socket directly.
type Player struct {
	sock string // explicit socket path; empty means the per-user default

	mu        sync.Mutex
	running   bool
	checkedAt time.Time
	gen       int // bumped on spawn so a stale reap can't clear the new state
}

// NewPlayer returns a player bound to sock. An empty sock means the default
// per-user socket path.
func NewPlayer(sock string) *Player { return &Player{sock: sock} }

// mpv is the player the UI drives. Tests replace it with one bound to a
// temporary socket.
var mpv = NewPlayer("")

func defaultMPVSocket() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "ytplay-mpv.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("ytplay-mpv-%d.sock", os.Getuid()))
}

func (p *Player) socket() string {
	if p.sock != "" {
		return p.sock
	}
	return defaultMPVSocket()
}

// Running reports whether a live mpv is listening on the socket. The probe is
// cached briefly so rapid redraws don't hammer the socket.
func (p *Player) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.checkedAt) < 250*time.Millisecond {
		return p.running
	}
	p.checkedAt = time.Now()
	conn, err := net.DialTimeout("unix", p.socket(), 100*time.Millisecond)
	if err != nil {
		p.running = false
		return false
	}
	_ = conn.Close()
	p.running = true
	return true
}

// reset clears the cached liveness state after a spawn or quit.
func (p *Player) reset() {
	p.mu.Lock()
	p.checkedAt = time.Time{}
	p.running = false
	p.mu.Unlock()
}

// command sends one command over a fresh connection and waits for its reply,
// skipping asynchronous events emitted by mpv scripts.
func (p *Player) command(args ...interface{}) error {
	if !p.Running() {
		return errMPVNotRunning
	}
	conn, err := net.DialTimeout("unix", p.socket(), 150*time.Millisecond)
	if err != nil {
		p.mu.Lock()
		p.running = false
		p.checkedAt = time.Now()
		p.mu.Unlock()
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(750 * time.Millisecond))
	return sendCommand(conn, 1, args)
}

func sendCommand(conn net.Conn, reqID int, args []interface{}) error {
	if err := json.NewEncoder(conn).Encode(map[string]interface{}{
		"command":    args,
		"request_id": reqID,
	}); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}
		var resp struct {
			RequestID int    `json:"request_id"`
			Error     string `json:"error"`
			Event     string `json:"event"`
		}
		if json.Unmarshal(line, &resp) != nil || resp.Event != "" {
			continue
		}
		if resp.RequestID == reqID || resp.RequestID == 0 {
			if resp.Error != "" && resp.Error != "success" {
				return fmt.Errorf("mpv: %s", resp.Error)
			}
			return nil
		}
	}
}

// Play makes urls the playlist and starts the first one immediately, unpausing
// if needed. If no mpv is running, it spawns one.
func (p *Player) Play(urls ...string) error {
	if len(urls) == 0 {
		return nil
	}
	// Probe freshly: a cached "not running" from a tick moments ago must not
	// make us spawn a second mpv alongside a live one.
	p.reset()
	if p.Running() {
		if err := p.replace(urls); err == nil {
			return nil
		}
		// The running instance refused the command; make sure it can't linger
		// as a second, uncontrolled player before starting fresh.
		_ = p.command("quit")
		p.reset()
	}
	if err := p.spawn(urls...); err != nil {
		return err
	}
	// Wait briefly for the socket so callers can rely on Running() immediately
	// after Play (mpv binds the IPC socket very early, so this is usually a
	// handful of milliseconds).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		p.reset()
		if p.Running() {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	return nil
}

func (p *Player) replace(urls []string) error {
	if err := p.command("loadfile", urls[0]); err != nil {
		return err
	}
	for _, u := range urls[1:] {
		if err := p.command("loadfile", u, "append"); err != nil {
			return err
		}
	}
	// mpv's pause property survives loadfile, so a "play now" must clear it or
	// a previously paused player shows a frozen first frame.
	_ = p.command("set", "pause", "no")
	return nil
}

// Enqueue appends urls to the running playlist and plays them after the current
// item. It is used when adding to a queue that is already playing.
func (p *Player) Enqueue(urls ...string) error {
	for _, u := range urls {
		if err := p.command("loadfile", u, "append-play"); err != nil {
			return err
		}
	}
	return nil
}

func (p *Player) spawn(urls ...string) error {
	sock := p.socket()
	_ = os.Remove(sock)

	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("mpv: %w", err)
	}
	defer devnull.Close()

	args := append([]string{"--input-ipc-server=" + sock}, urls...)
	cmd := exec.Command("mpv", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Detach mpv from the TUI's streams: stdin would otherwise receive the
	// keystrokes the TUI is listening for, and mpv's own output would print
	// into the alternate screen and desync the cell-anchored image layout.
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mpv: %w", err)
	}

	p.mu.Lock()
	p.gen++
	gen := p.gen
	p.mu.Unlock()

	// Reap the detached process so it doesn't linger as a zombie, and clear the
	// cached liveness only if this is still the current generation.
	go func() {
		_ = cmd.Wait()
		p.mu.Lock()
		if p.gen == gen {
			p.running = false
			p.checkedAt = time.Now()
		}
		p.mu.Unlock()
	}()
	p.reset()
	return nil
}

// PlaylistPos returns the 0-based index of the current playlist entry, or -1.
func (p *Player) PlaylistPos() int {
	if !p.Running() {
		return -1
	}
	conn, err := net.DialTimeout("unix", p.socket(), 150*time.Millisecond)
	if err != nil {
		return -1
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	if err := json.NewEncoder(conn).Encode(map[string]interface{}{
		"command":    []interface{}{"get_property", "playlist-pos"},
		"request_id": 1,
	}); err != nil {
		return -1
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return -1
		}
		var resp struct {
			RequestID int         `json:"request_id"`
			Data      interface{} `json:"data"`
			Event     string      `json:"event"`
		}
		if json.Unmarshal(line, &resp) != nil || resp.Event != "" || resp.RequestID != 1 {
			continue
		}
		switch v := resp.Data.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
		return -1
	}
}

// Transport / playlist commands. They return errMPVNotRunning when no mpv is up.
func (p *Player) TogglePause() error   { return p.command("cycle", "pause") }
func (p *Player) Next() error          { return p.command("playlist-next") }
func (p *Player) Prev() error          { return p.command("playlist-prev") }
func (p *Player) Volume(d int) error   { return p.command("add", "volume", d) }
func (p *Player) ToggleMute() error    { return p.command("cycle", "mute") }
func (p *Player) RemoveAt(i int) error { return p.command("playlist-remove", i) }
func (p *Player) Move(from, to int) error {
	return p.command("playlist-move", from, to)
}
func (p *Player) Clear() error          { return p.command("playlist-clear") }
func (p *Player) PlayIndex(i int) error { return p.command("playlist-play-index", i) }
func (p *Player) Quit() error           { return p.command("quit") }

// mpvControlMsg reports the outcome of a transport command so the TUI can show
// feedback.
type mpvControlMsg struct {
	label string
	err   error
}

// mpvControlCmd runs one mpv command off the UI goroutine and reports back.
func mpvControlCmd(label string, args ...interface{}) tea.Cmd {
	return func() tea.Msg {
		if !mpv.Running() {
			return mpvControlMsg{label: label, err: errMPVNotRunning}
		}
		if err := mpv.command(args...); err != nil {
			return mpvControlMsg{label: label, err: err}
		}
		return mpvControlMsg{label: label}
	}
}
