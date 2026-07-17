package insightbatchcreate

import (
	"fmt"
	"io"
	"strings"
	"time"

	"encoding/base64"

	"golang.org/x/crypto/ssh"
)

// hostSysInfo holds system info collected from a single host via SSH.
type hostSysInfo struct {
	IP    string
	MemGB int
	Virt  string // "none" = physical, otherwise VM type
}

// memToServerType maps memory (GB) and virtualization to server_type.
// VM:  <24 → error (or vm_l if allowLowMemVM), [24,32) → vm_l, [32,48) → vm_m, >=48 → vm_h
// PM:  always → pm
func memToServerType(memGB int, virt string, allowLowMemVM bool) (string, error) {
	if virt == "none" {
		return "pm", nil
	}
	if memGB < 24 {
		if allowLowMemVM {
			return "vm_l", nil
		}
		return "", fmt.Errorf("虚拟机内存不足: %dG < 24G 最低要求", memGB)
	}
	if memGB < 32 {
		return "vm_l", nil
	}
	if memGB < 48 {
		return "vm_m", nil
	}
	return "vm_h", nil
}

// detectHostSysInfo connects to a host via SSH and collects memory + virtualization info.
func detectHostSysInfo(ip string, port int, user, passwordB64 string, timeout time.Duration) (hostSysInfo, error) {
	info := hostSysInfo{IP: ip}

	password, err := decodeB64(passwordB64)
	if err != nil {
		return info, fmt.Errorf("解码 SSH 密码: %w", err)
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
		return info, fmt.Errorf("SSH 连接 %s 失败: %w", addr, err)
	}
	defer conn.Close()

	// Memory (total, GB)
	out, err := runSSHCmd(conn, "free -g | awk '/^Mem:/{print $2}'")
	if err != nil {
		return info, fmt.Errorf("查询内存失败 %s: %w", ip, err)
	}
	info.MemGB = parseInt(out)

	// Virtualization
	virtOut, err := runSSHCmd(conn, "systemd-detect-virt --vm")
	if err != nil {
		// Fallback: check DMI product name
		virtOut, err = runSSHCmd(conn, "dmidecode -s system-product-name 2>/dev/null")
		if err != nil || strings.TrimSpace(virtOut) == "" {
			info.Virt = "unknown"
		} else {
			info.Virt = strings.TrimSpace(strings.ToLower(virtOut))
			if info.Virt == "none" || info.Virt == "" {
				info.Virt = "none"
			}
		}
	} else {
		virt := strings.TrimSpace(strings.ToLower(virtOut))
		if virt == "none" || virt == "" {
			info.Virt = "none"
		} else {
			info.Virt = virt
		}
	}

	return info, nil
}

// resolveClusterServerType SSH-detects server_type for a cluster.
// All IPs must succeed SSH and yield the same server_type, otherwise returns an error.
func resolveClusterServerType(ips []string, port int, user, passwordB64 string, timeout time.Duration, allowLowMemVM bool) (string, error) {
	if len(ips) == 0 {
		return "", fmt.Errorf("集群无有效 IP 地址")
	}

	var detectedType string
	for _, ip := range ips {
		info, err := detectHostSysInfo(ip, port, user, passwordB64, timeout)
		if err != nil {
			return "", err
		}

		st, err := memToServerType(info.MemGB, info.Virt, allowLowMemVM)
		if err != nil {
			return "", fmt.Errorf("%s: %w", ip, err)
		}

		if detectedType == "" {
			detectedType = st
		} else if detectedType != st {
			return "", fmt.Errorf("同集群检测到不同类型: 已有 %s, %s 检测为 %s (内存 %dG, 虚拟化 %s)",
				detectedType, ip, st, info.MemGB, info.Virt)
		}
	}

	return detectedType, nil
}

func runSSHCmd(conn *ssh.Client, cmd string) (string, error) {
	sess, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("create session failed: %w", err)
	}
	defer sess.Close()

	out, err := sess.StdoutPipe()
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
	case <-time.After(15 * time.Second):
		sess.Close()
		return "", fmt.Errorf("command timed out: %s", cmd)
	}

	data, err := io.ReadAll(out)
	if err != nil {
		return "", fmt.Errorf("read stdout failed: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func decodeB64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s, nil // not base64 → treat as plain text
	}
	return string(b), nil
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
