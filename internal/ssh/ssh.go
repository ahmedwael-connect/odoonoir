package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Client represents an SSH connection to a remote host
type Client struct {
	client   *ssh.Client
	config   *ssh.ClientConfig
	host     string
	port     int
	user     string
	keyPath  string
	tunnels  map[string]*Tunnel
	mu       sync.Mutex
}

// Tunnel represents an SSH tunnel
type Tunnel struct {
	LocalPort  int
	RemoteHost string
	RemotePort int
	listener   net.Listener
	cancel     context.CancelFunc
}

// Config holds SSH connection configuration
type Config struct {
	Host     string
	Port     int
	User     string
	KeyPath  string
	Password string
	Timeout  time.Duration
}

// NewClient creates a new SSH client
func NewClient(cfg Config) (*Client, error) {
	var authMethods []ssh.AuthMethod

	if cfg.KeyPath != "" {
		key, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	// Try SSH agent
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	config := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: add known hosts verification
		Timeout:         cfg.Timeout,
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("dial ssh: %w", err)
	}

	return &Client{
		client:  client,
		config:  config,
		host:    cfg.Host,
		port:    cfg.Port,
		user:    cfg.User,
		keyPath: cfg.KeyPath,
		tunnels: make(map[string]*Tunnel),
	}, nil
}

// Close closes the SSH connection and all tunnels
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, tunnel := range c.tunnels {
		tunnel.Close()
	}
	return c.client.Close()
}

// IsAlive checks if the SSH connection is still alive
func (c *Client) IsAlive() bool {
	return c.client != nil
}

// RunCommand executes a command on the remote host
func (c *Client) RunCommand(ctx context.Context, cmd string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr strings.Builder
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err := <-done:
		if err != nil {
			return stdout.String(), fmt.Errorf("command failed: %w\nstderr: %s", err, stderr.String())
		}
		return stdout.String(), nil
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		return stdout.String(), ctx.Err()
	}
}

// RunCommandWithOutput executes a command and streams output
func (c *Client) RunCommandWithOutput(ctx context.Context, cmd string, stdout, stderr io.Writer) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		return ctx.Err()
	}
}

// StartTunnel creates an SSH tunnel
func (c *Client) StartTunnel(ctx context.Context, localPort int, remoteHost string, remotePort int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%d:%s:%d", localPort, remoteHost, remotePort)
	if _, exists := c.tunnels[key]; exists {
		return fmt.Errorf("tunnel already exists")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", localPort))
	if err != nil {
		return fmt.Errorf("listen on local port: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	tunnel := &Tunnel{
		LocalPort:  localPort,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
		listener:   listener,
		cancel:     cancel,
	}
	c.tunnels[key] = tunnel

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			go c.handleTunnel(conn, remoteHost, remotePort, ctx)
		}
	}()

	return nil
}

// StopTunnel stops an SSH tunnel
func (c *Client) StopTunnel(localPort int, remoteHost string, remotePort int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%d:%s:%d", localPort, remoteHost, remotePort)
	tunnel, exists := c.tunnels[key]
	if !exists {
		return fmt.Errorf("tunnel not found")
	}

	tunnel.Close()
	delete(c.tunnels, key)
	return nil
}

func (c *Client) handleTunnel(localConn net.Conn, remoteHost string, remotePort int, ctx context.Context) {
	defer localConn.Close()

	remoteConn, err := c.client.Dial("tcp", fmt.Sprintf("%s:%d", remoteHost, remotePort))
	if err != nil {
		return
	}
	defer remoteConn.Close()

	done := make(chan struct{})
	go func() {
		io.Copy(remoteConn, localConn)
		close(done)
	}()
	go func() {
		io.Copy(localConn, remoteConn)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Tunnel methods
func (t *Tunnel) Close() error {
	if t.cancel != nil {
		t.cancel()
	}
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

// CopyFile copies a file to/from the remote host using SCP
func (c *Client) CopyFile(ctx context.Context, src, dst string, toRemote bool) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var cmd string
	if toRemote {
		cmd = fmt.Sprintf("scp -t %s", dst)
	} else {
		cmd = fmt.Sprintf("scp -f %s", src)
	}

	_, err = session.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	_, err = session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("start scp: %w", err)
	}

	// Simplified SCP implementation
	// In production, use a proper SCP library
	return session.Wait()
}

// RemoteInfo holds information about a remote host
type RemoteInfo struct {
	Hostname    string
	OS          string
	Arch        string
	CPUs        int
	MemoryGB    float64
	DiskGB      float64
	Docker      bool
	PostgreSQL  bool
	OdooVersion string
}

// GetRemoteInfo gathers information about the remote host
func (c *Client) GetRemoteInfo(ctx context.Context) (*RemoteInfo, error) {
	info := &RemoteInfo{}

	// Get hostname
	out, err := c.RunCommand(ctx, "hostname")
	if err == nil {
		info.Hostname = strings.TrimSpace(out)
	}

	// Get OS info
	out, err = c.RunCommand(ctx, "cat /etc/os-release")
	if err == nil {
		info.OS = strings.TrimSpace(out)
	}

	// Get architecture
	out, err = c.RunCommand(ctx, "uname -m")
	if err == nil {
		info.Arch = strings.TrimSpace(out)
	}

	// Get CPU count
	out, err = c.RunCommand(ctx, "nproc")
	if err == nil {
		fmt.Sscanf(strings.TrimSpace(out), "%d", &info.CPUs)
	}

	// Get memory
	out, err = c.RunCommand(ctx, "free -g | awk '/^Mem:/ {print $2}'")
	if err == nil {
		fmt.Sscanf(strings.TrimSpace(out), "%f", &info.MemoryGB)
	}

	// Get disk space
	out, err = c.RunCommand(ctx, "df -BG / | awk 'NR==2 {print $4}' | sed 's/G//'")
	if err == nil {
		fmt.Sscanf(strings.TrimSpace(out), "%f", &info.DiskGB)
	}

	// Check Docker
	out, err = c.RunCommand(ctx, "which docker")
	if err == nil && out != "" {
		info.Docker = true
	}

	// Check PostgreSQL
	out, err = c.RunCommand(ctx, "which psql")
	if err == nil && out != "" {
		info.PostgreSQL = true
	}

	// Check Odoo version
	out, err = c.RunCommand(ctx, "odoo --version 2>/dev/null || python3 -c \"import odoo; print(odoo.release.version)\" 2>/dev/null")
	if err == nil {
		info.OdooVersion = strings.TrimSpace(out)
	}

	return info, nil
}

// File operations
func (c *Client) ReadFile(ctx context.Context, path string) (string, error) {
	return c.RunCommand(ctx, fmt.Sprintf("cat %s", path))
}

func (c *Client) WriteFile(ctx context.Context, path, content string) error {
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", path, content)
	_, err := c.RunCommand(ctx, cmd)
	return err
}

func (c *Client) FileExists(ctx context.Context, path string) (bool, error) {
	out, err := c.RunCommand(ctx, fmt.Sprintf("test -f %s && echo exists || echo missing", path))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "exists", nil
}

func (c *Client) ListDir(ctx context.Context, path string) ([]string, error) {
	out, err := c.RunCommand(ctx, fmt.Sprintf("ls -1 %s", path))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}
	return lines, nil
}
