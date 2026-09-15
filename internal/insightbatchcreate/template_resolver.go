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
// Accounts for free -g rounding: a 32G server reports ~31G.
// VM:  <22 → error (or vm_l when allowLowMemVM), [22,30) → vm_l,
// [30,46) → vm_m, >=46 → vm_h.
// PM:  always → pm
func memToServerType(memGB int, virt string, allowLowMemVM bool) (string, error) {
	if virt == "none" {
		return "pm", nil
	}
	if memGB < lowMemoryVMThresholdGB {
		if allowLowMemVM {
			return "vm_l", nil
		}
		return "", fmt.Errorf("虚拟机内存不足: %dG < %dG 最低要求", memGB, lowMemoryVMThresholdGB)
	}
	if memGB < 30 {
		return "vm_l", nil
	}
	if memGB < 46 {
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
// All IPs must succeed SSH. By default they must yield the same server_type.
// When allowMismatch is true, a type mismatch is allowed with a warning. If
// the mismatch includes a low-memory VM, vm_l is used as the template type.
func resolveClusterServerType(ips []string, port int, user string, auth *hostchecker.SSHAuth, timeout time.Duration, allowLowMemVM, allowMismatch bool) (string, error) {
	if len(ips) == 0 {
		return "", fmt.Errorf("集群无有效 IP 地址")
	}

	var detectedType string
	lowMemoryVM := false
	typeMismatch := false
	var mismatchIP, mismatchType string
	for _, ip := range ips {
		log.Printf("[模版检测] 正在检测 %s...", ip)
		info, err := detectHostSysInfo(ip, port, user, auth, timeout)
		if err != nil {
			return "", err
		}

		isLowMemoryVM := info.Virt != "none" && info.MemGB < lowMemoryVMThresholdGB
		var st string
		if isLowMemoryVM && !allowLowMemVM {
			// Keep the original threshold failure unless an allowed cluster-level
			// mismatch is established after all hosts are inspected.
			st = "vm_l"
			lowMemoryVM = true
		} else {
			st, err = memToServerType(info.MemGB, info.Virt, allowLowMemVM)
			if err != nil {
				return "", fmt.Errorf("%s: %w", ip, err)
			}
		}

		log.Printf("[模版检测] %s: 内存=%dG 虚拟化=%s → server_type=%s", ip, info.MemGB, info.Virt, st)

		if detectedType == "" {
			detectedType = st
		} else if detectedType != st {
			typeMismatch = true
			if mismatchIP == "" {
				mismatchIP = ip
				mismatchType = st
			}
		}
	}

	if typeMismatch && allowMismatch {
		log.Printf("[模版检测] 告警：集群主机 server_type 不一致，%s 实际为 %s，首个主机实际值为 %s (--allow-server-type-mismatch)", mismatchIP, mismatchType, detectedType)
	}
	if lowMemoryVM && typeMismatch && allowMismatch {
		log.Printf("[模版检测] 告警：集群存在内存低于 %dG 的虚拟机且 server_type 不一致，使用 vm_l 模板 (--allow-server-type-mismatch)", lowMemoryVMThresholdGB)
	}

	selected, err := selectClusterServerType(detectedType, mismatchIP != "", lowMemoryVM, allowMismatch)
	if err != nil {
		if lowMemoryVM {
			return "", err
		}
		return "", fmt.Errorf("同集群检测到不同类型: 已有 %s, %s 检测为 %s (--allow-server-type-mismatch 可仅告警继续)", detectedType, mismatchIP, mismatchType)
	}
	return selected, nil
}

// selectClusterServerType applies the cluster-level compatibility rules after
// every host has been inspected. Low-memory VMs remain an error unless they
// are part of an allowed server_type mismatch; that special case selects vm_l.
func selectClusterServerType(detectedType string, typeMismatch, lowMemoryVM, allowMismatch bool) (string, error) {
	if lowMemoryVM {
		if !allowMismatch || !typeMismatch {
			return "", fmt.Errorf("集群存在虚拟机内存不足: 低于 %dG；只有在 server_type 不一致并指定 --allow-server-type-mismatch 时才允许使用 vm_l 模板", lowMemoryVMThresholdGB)
		}
		return "vm_l", nil
	}
	if typeMismatch && !allowMismatch {
		return "", fmt.Errorf("同集群检测到不同类型")
	}
	return detectedType, nil
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
