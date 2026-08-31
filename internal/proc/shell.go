package proc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// ShellSession represents an interactive odoo shell session with PTY.
type ShellSession struct {
	conn     *websocket.Conn
	cmd      *exec.Cmd
	pty      *os.File
	stdin    io.WriteCloser
	stdout   io.Reader
	stderr   io.Reader
	done     chan struct{}
	mu       sync.Mutex
	resizeCh chan PtySize
}

// PtySize represents terminal dimensions.
type PtySize struct {
	Rows uint16
	Cols uint16
}

// NewShellSession spawns `odoo shell -c conf -d db` with PTY.
func NewShellSession(inst *instance.Instance, p instance.Paths, py, dbName string) (*ShellSession, error) {
	if _, err := os.Stat(p.Source); err != nil {
		return nil, fmt.Errorf("odoo source not found at %s — check instance source path", p.Source)
	}
	if _, err := os.Stat(filepath.Join(p.Source, "odoo-bin")); err != nil {
		return nil, fmt.Errorf("odoo-bin not found at %s/odoo-bin — source may be empty or not cloned", p.Source)
	}
	if _, err := os.Stat(p.Conf); err != nil {
		return nil, fmt.Errorf("odoo.conf not found at %s", p.Conf)
	}
	if _, err := exec.LookPath(py); err != nil {
		if _, err2 := os.Stat(py); err2 != nil {
			return nil, fmt.Errorf("python not found: %s — check venv bin/python", py)
		}
	}
	args := []string{filepath.Join(p.Source, "odoo-bin"), "shell", "-c", p.Conf}
	if dbName != "" {
		args = append(args, "-d", dbName)
	}
	cmd := exec.Command(py, args...)
	cmd.Dir = p.Source
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start the command with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	s := &ShellSession{
		cmd:      cmd,
		pty:      ptmx,
		stdin:    ptmx,
		stdout:   ptmx,
		stderr:   ptmx,
		done:     make(chan struct{}),
		resizeCh: make(chan PtySize, 1),
	}

	// Handle process exit
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()

	return s, nil
}

// HandleWS upgrades HTTP to WebSocket and pipes stdin/stdout/stderr.
func (s *ShellSession) HandleWS(w http.ResponseWriter, r *http.Request) error {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	s.conn = conn

	// Configure PTY for raw mode
	if err := pty.InheritSize(os.Stdin, s.pty); err != nil {
		// Ignore if not a TTY
	}

	// Start reading from WebSocket -> PTY stdin
	go s.readWS()

	// Start reading from PTY stdout -> WebSocket
	go s.writeWS()

	// Wait for done
	<-s.done
	_ = conn.Close()
	return nil
}

// readWS reads messages from WebSocket and writes to PTY stdin.
func (s *ShellSession) readWS() {
	defer close(s.done)
	for {
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			return
		}

		// Handle resize messages: {"type":"resize","rows":24,"cols":80}
		if len(msg) > 0 && msg[0] == '{' {
			var resizeMsg struct {
				Type string `json:"type"`
				Rows uint16 `json:"rows"`
				Cols uint16 `json:"cols"`
			}
			if json.Unmarshal(msg, &resizeMsg) == nil && resizeMsg.Type == "resize" {
				s.resizeCh <- PtySize{Rows: resizeMsg.Rows, Cols: resizeMsg.Cols}
				continue
			}
		}

		// Write to PTY stdin
		s.mu.Lock()
		_, err = s.stdin.Write(msg)
		s.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// writeWS reads from PTY stdout and writes to WebSocket.
func (s *ShellSession) writeWS() {
	buf := make([]byte, 4096)
	for {
		n, err := s.stdout.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Try to send error message
				_ = s.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n[Shell error: %v]\r\n", err)))
			}
			return
		}
		if n > 0 {
			s.mu.Lock()
			err := s.conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}

		// Check for resize
		select {
		case sz := <-s.resizeCh:
			_ = pty.Setsize(s.pty, &pty.Winsize{Rows: sz.Rows, Cols: sz.Cols})
		default:
		}
	}
}

// Resize updates the PTY window size.
func (s *ShellSession) Resize(rows, cols uint16) {
	select {
	case s.resizeCh <- PtySize{Rows: rows, Cols: cols}:
	default:
	}
}

// Close terminates the shell session.
func (s *ShellSession) Close() error {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
	if s.pty != nil {
		_ = s.pty.Close()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	return nil
}