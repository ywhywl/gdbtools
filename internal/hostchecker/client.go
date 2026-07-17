package hostchecker

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client wraps an SSH connection for executing remote commands.
type Client struct {
	conn    *ssh.Client
	timeout time.Duration
}

// NewClient creates an SSH client and connects to the host.
func NewClient(ip string, port int, user string, passwordB64 string, timeout time.Duration) (*Client, error) {
	password, err := decodeBase64(passwordB64)
	if err != nil {
		return nil, fmt.Errorf("decode SSH password: %w", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         timeout,
	}

	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH connect %s failed: %w", addr, err)
	}

	return &Client{conn: conn, timeout: timeout}, nil
}

// Run executes a command and returns stdout (trimmed).
func (c *Client) Run(cmd string) (string, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("create session failed: %w", err)
	}
	defer sess.Close()

	var stdout io.Reader
	stdout, err = sess.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe failed: %w", err)
	}

	if err := sess.Start(cmd); err != nil {
		return "", fmt.Errorf("start command failed: %w", err)
	}

	done := make(chan struct{})
	go func() {
		sess.Wait()
		close(done)
	}()

	select {
	case <-done:
		// finished
	case <-time.After(c.timeout):
		sess.Close()
		return "", fmt.Errorf("command timed out after %v: %s", c.timeout, cmd)
	}

	out, err := io.ReadAll(stdout)
	if err != nil {
		return "", fmt.Errorf("read stdout failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RunWithFallback tries cmd first, and if it fails, tries fallback.
func (c *Client) RunWithFallback(cmd, fallback string) (string, error) {
	out, err := c.Run(cmd)
	if err == nil && out != "" {
		return out, nil
	}
	return c.Run(fallback)
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func decodeBase64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// not base64 — treat as plain text
		return s, nil
	}
	return string(b), nil
}
