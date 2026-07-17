package hostchecker

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHAuth holds resolved SSH authentication configuration.
// Priority: SSH Agent > private key > password.
type SSHAuth struct {
	AgentSock string // SSH_AUTH_SOCK path, empty if not available
	KeyPath   string // resolved private key path, empty if none found
	KeyData   []byte // key file content
	Password  string // password fallback (may be empty)
}

// NewSSHAuth resolves SSH authentication from inputs.
// If keyPath is non-empty, uses it; otherwise auto-discovers default key files.
// Password is used as fallback when no key is available.
func NewSSHAuth(keyPath, password, passwordB64 string) (*SSHAuth, error) {
	auth := &SSHAuth{}

	// Resolve SSH agent
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" && sock != "/dev/null" && sock != "none" {
		if _, err := os.Stat(sock); err == nil {
			auth.AgentSock = sock
		}
	}

	// Resolve password (always set, may be empty)
	if strings.TrimSpace(passwordB64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(passwordB64))
		if err != nil {
			return nil, fmt.Errorf("decode SSH password: %w", err)
		}
		auth.Password = strings.TrimSpace(string(decoded))
	} else if strings.TrimSpace(password) != "" {
		auth.Password = strings.TrimSpace(password)
	}

	// Resolve key file
	keyPath = strings.TrimSpace(keyPath)
	if keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read key file %s: %w", keyPath, err)
		}
		auth.KeyPath = keyPath
		auth.KeyData = data
		return auth, nil
	}

	// Auto-discover default key files
	home, _ := os.UserHomeDir()
	if home != "" {
		candidates := []string{
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "id_rsa"),
			filepath.Join(home, ".ssh", "id_ecdsa"),
			filepath.Join(home, ".ssh", "id_dsa"),
		}
		for _, p := range candidates {
			if data, err := os.ReadFile(p); err == nil {
				auth.KeyPath = p
				auth.KeyData = data
				return auth, nil
			}
		}
	}

	return auth, nil
}

// HasAuth returns true if at least one auth method is available.
func (a *SSHAuth) HasAuth() bool {
	return a.AgentSock != "" || a.KeyPath != "" || a.Password != ""
}

// buildAuthMethods constructs ssh.AuthMethod list from resolved auth config.
func (a *SSHAuth) buildAuthMethods() []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	// 1. SSH agent (highest priority)
	if a.AgentSock != "" {
		conn, err := net.Dial("unix", a.AgentSock)
		if err == nil {
			agentClient := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	// 2. Private key file
	if a.KeyData != nil {
		signer, err := parsePrivateKey(a.KeyData, a.Password)
		if err == nil {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}

	// 3. Password fallback
	if a.Password != "" {
		methods = append(methods, ssh.Password(a.Password))
	}

	return methods
}

// Client wraps an SSH connection for executing remote commands.
type Client struct {
	conn    *ssh.Client
	timeout time.Duration
}

// NewClient creates an SSH client and connects to the host using SSHAuth.
func NewClient(ip string, port int, user string, auth *SSHAuth, timeout time.Duration) (*Client, error) {
	authMethods := auth.buildAuthMethods()
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth available (need key, agent, or password)")
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
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

// DetectVirt determines virtualization type using DMI + CPU hypervisor flag.
// Returns "none" for physical machines, or the VM type string (e.g. "kvm", "vmware", "xen").
// DMI product name is checked first; if it contains known virtualization keywords, returns VM.
// CPU hypervisor flag (/proc/cpuinfo) is used as secondary confirmation — if DMI is ambiguous
// but the flag exists, it's a VM. If neither triggers, it's a physical machine.
func (c *Client) DetectVirt() string {
	// 1. DMI product name
	dmiOut, dmiErr := c.Run("cat /sys/class/dmi/id/product_name 2>/dev/null")
	dmiProduct := strings.TrimSpace(strings.ToLower(dmiOut))

	// 2. CPU hypervisor flag (x86 only, but harmless on aarch64)
	hasHypervisor := false
	if out, err := c.Run("grep -qw hypervisor /proc/cpuinfo 2>/dev/null && echo yes || echo no"); err == nil {
		hasHypervisor = (out == "yes")
	}

	// DMI-based VM detection keywords
	vmKeywords := []string{
		"kvm", "qemu", "vmware", "virtualbox", "xen", "hyper-v",
		"bochs", "parallels", "virtuozzo", "alc",
		"openstack", "proxmox", "nutanix",
		// 国产云厂商虚拟机
		"cvm",   // 腾讯云 Cloud Virtual Machine
		"ecs",   // 阿里云 Elastic Compute Service
		"bcc",   // 百度云 Baidu Cloud Compute
		"cloud", // 通用云服务器标识
	}

	// Clear physical machine indicators (DMI values that confirm PM)
	pmKeywords := []string{
		"dell", "hp ", "hpe ", "lenovo", "ibm", "supermicro",
		"inspur", "huawei", "sugon", "greatwall",
		"tencent", "alibaba", "amazon", "microsoft",
	}

	isVMByDMI := false
	if dmiErr == nil && dmiProduct != "" {
		for _, kw := range vmKeywords {
			if strings.Contains(dmiProduct, kw) {
				isVMByDMI = true
				break
			}
		}
	}

	// If DMI shows a physical machine brand, it's definitely a PM
	if dmiErr == nil && dmiProduct != "" {
		for _, kw := range pmKeywords {
			if strings.Contains(dmiProduct, kw) {
				return "none"
			}
		}
	}

	// CPU hypervisor flag is definitive — if present, it's a VM
	if hasHypervisor {
		if isVMByDMI {
			return dmiProduct
		}
		return "hypervisor"
	}

	// DMI detected a VM keyword but no CPU flag — still treat as VM
	// (aarch64 has no hypervisor flag, or flag not set)
	if isVMByDMI {
		return dmiProduct
	}

	return "none"
}

// parsePrivateKey tries to parse a PEM-encoded private key.
// For encrypted keys, it tries the provided password.
func parsePrivateKey(keyData []byte, password string) (ssh.Signer, error) {
	block, _ := pem.Decode(keyData)
	if block == nil {
		// Try OpenSSH format (ed25519 etc.)
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, err
		}
		return signer, nil
	}

	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck // legacy RFC 1423 format, handled for compatibility
		if password == "" {
			return nil, fmt.Errorf("key is encrypted but no password provided")
		}
		decoded, err := x509.DecryptPEMBlock(block, []byte(password)) //nolint:staticcheck
		if err != nil {
			return nil, fmt.Errorf("decrypt key failed: %w", err)
		}
		return ssh.ParsePrivateKey(decoded)
	}

	return ssh.ParsePrivateKey(block.Bytes)
}
