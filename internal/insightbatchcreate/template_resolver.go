package insightbatchcreate

import (
	"fmt"
	"log"
	"time"

	"github.com/ywhywl/gdbtools/internal/hostchecker"
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
func detectHostSysInfo(ip string, port int, user string, auth *hostchecker.SSHAuth, timeout time.Duration) (hostSysInfo, error) {
	info := hostSysInfo{IP: ip}

	c, err := hostchecker.NewClient(ip, port, user, auth, timeout)
	if err != nil {
		return info, fmt.Errorf("SSH 连接 %s 失败: %w", ip, err)
	}
	defer c.Close()

	// Memory (total, GB)
	out, err := c.Run("free -g | awk '/^Mem:/{print $2}'")
	if err != nil {
		return info, fmt.Errorf("查询内存失败 %s: %w", ip, err)
	}
	info.MemGB = parseInt(out)

	// Virtualization (DMI + CPU hypervisor flag)
	info.Virt = c.DetectVirt()

	return info, nil
}

// resolveClusterServerType SSH-detects server_type for a cluster.
// All IPs must succeed SSH and yield the same server_type, otherwise returns an error.
func resolveClusterServerType(ips []string, port int, user string, auth *hostchecker.SSHAuth, timeout time.Duration, allowLowMemVM bool) (string, error) {
	if len(ips) == 0 {
		return "", fmt.Errorf("集群无有效 IP 地址")
	}

	var detectedType string
	for _, ip := range ips {
		log.Printf("[模版检测] 正在检测 %s...", ip)
		info, err := detectHostSysInfo(ip, port, user, auth, timeout)
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

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
