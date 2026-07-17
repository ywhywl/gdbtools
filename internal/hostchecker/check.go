package hostchecker

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// CheckAll checks all hosts via SSH and returns results.
func CheckAll(hosts []map[string]string, sshPort int, sshUser, sshPasswordB64 string, timeout time.Duration) []CheckResult {
	results := make([]CheckResult, 0, len(hosts))
	for _, host := range hosts {
		ip := strings.TrimSpace(host["server_ip"])
		if ip == "" {
			continue
		}
		log.Printf("[check] 检查主机: %s", ip)

		result, err := checkHost(ip, sshPort, sshUser, sshPasswordB64, timeout)
		if err != nil {
			result = CheckResult{
				IP:      ip,
				Passed:  false,
				Reasons: []string{err.Error()},
				SysInfo: &SysInfo{IP: ip},
			}
		}
		results = append(results, result)

		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}
		log.Printf("[check] %s %s: os=%s arch=%s virt=%s cpu=%d mem=%dG data_mount=%v data_avail=%dG",
			status, ip, result.SysInfo.OS, result.SysInfo.CPUArch, result.SysInfo.Virt,
			result.SysInfo.CPU, result.SysInfo.MemGB, result.SysInfo.HasData, result.SysInfo.DataAvailGB)
	}
	return results
}

func checkHost(ip string, sshPort int, sshUser, sshPasswordB64 string, timeout time.Duration) (CheckResult, error) {
	c, err := NewClient(ip, sshPort, sshUser, sshPasswordB64, timeout)
	if err != nil {
		return CheckResult{IP: ip, SysInfo: &SysInfo{IP: ip}}, err
	}
	defer c.Close()

	sysInfo, err := collectSysInfo(c, ip)
	if err != nil {
		return CheckResult{IP: ip, SysInfo: &sysInfo}, err
	}

	reasons := evaluateRules(sysInfo)

	resolvedDataPath, resolvedInstallPath := resolvePaths(sysInfo.HasData, sysInfo.Virt)

	return CheckResult{
		IP:                  ip,
		Passed:              len(reasons) == 0,
		Reasons:             reasons,
		SysInfo:             &sysInfo,
		ResolvedDataPath:    resolvedDataPath,
		ResolvedInstallPath: resolvedInstallPath,
	}, nil
}

func collectSysInfo(c *Client, ip string) (SysInfo, error) {
	info := SysInfo{IP: ip}

	// 1. CPU architecture
	out, err := c.Run("uname -m")
	if err != nil {
		return info, fmt.Errorf("uname -m failed: %w", err)
	}
	info.CPUArch = out

	// 2. CPU cores
	out, err = c.Run("nproc")
	if err != nil {
		return info, fmt.Errorf("nproc failed: %w", err)
	}
	if val, parseErr := strconv.Atoi(out); parseErr == nil {
		info.CPU = val
	}

	// 3. Memory in GB
	out, err = c.Run("free -g | awk '/^Mem:/{print $2}'")
	if err != nil {
		return info, fmt.Errorf("free -g failed: %w", err)
	}
	if val, parseErr := strconv.Atoi(out); parseErr == nil {
		info.MemGB = val
	}

	// 4. /data mount check
	_, err = c.Run("df -BG /data")
	info.HasData = (err == nil)
	if info.HasData {
		// get available space
		out2, err2 := c.Run("df -BG /data | awk 'NR==2{gsub(/G/,\"\",$4); print $4}'")
		if err2 == nil {
			if val, parseErr := strconv.Atoi(out2); parseErr == nil {
				info.DataAvailGB = val
			}
		}
	}

	// 5. OS detection
	osID, err := c.RunWithFallback("grep -oP '(?<=^ID=).+' /etc/os-release", "cat /etc/os-release")
	if err != nil {
		// fallback: check release files
		osID = detectOSFromReleaseFiles(c)
	} else {
		osID = strings.TrimSpace(strings.ToLower(osID))
	}
	info.OS = normalizeOS(osID)

	// 6. Virtualization detection
	virtOut, err := c.RunWithFallback("systemd-detect-virt --vm", "dmidecode -s system-product-name")
	if err != nil {
		info.Virt = "unknown"
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

func detectOSFromReleaseFiles(c *Client) string {
	// Try kylin-release first, then centos-release
	if out, err := c.Run("cat /etc/kylin-release 2>/dev/null"); err == nil && out != "" {
		return out
	}
	if out, err := c.Run("cat /etc/centos-release 2>/dev/null"); err == nil && out != "" {
		return out
	}
	return ""
}

func normalizeOS(id string) string {
	lower := strings.ToLower(id)
	if strings.Contains(lower, "kylin") || strings.Contains(lower, "neokylin") {
		return OSKylin
	}
	if strings.Contains(lower, "centos") {
		return OSCentOS
	}
	if lower == "" {
		return "unknown"
	}
	return lower
}

func evaluateRules(info SysInfo) []string {
	var reasons []string

	// OS check — must be Kylin
	if info.OS == OSCentOS {
		reasons = append(reasons, "操作系统为 CentOS，仅支持麒麟系统")
	} else if info.OS != OSKylin {
		reasons = append(reasons, fmt.Sprintf("操作系统为 %s，仅支持麒麟系统", info.OS))
	}

	if info.Virt == "none" {
		// Physical machine rules
		if !info.HasData {
			reasons = append(reasons, "/data 目录未挂载")
		} else if info.DataAvailGB < PhysDataAvailMin {
			reasons = append(reasons, fmt.Sprintf("/data 可用空间不足: %dG < %dG (3T)", info.DataAvailGB, PhysDataAvailMin))
		}
		if info.CPU < PhysCPUMin {
			reasons = append(reasons, fmt.Sprintf("CPU 核心数不足: %d < %d", info.CPU, PhysCPUMin))
		}
		if info.MemGB < PhysMemMin {
			reasons = append(reasons, fmt.Sprintf("内存不足: %dG < %dG", info.MemGB, PhysMemMin))
		}
	} else {
		// VM rules
		if info.HasData {
			reasons = append(reasons, "虚拟机不应挂载 /data 目录")
		}
		if info.CPU > VMCPUMax {
			reasons = append(reasons, fmt.Sprintf("VM CPU 核心数过多: %d > %d", info.CPU, VMCPUMax))
		}
		if info.MemGB < VMMemMin || info.MemGB > VMMemMax {
			reasons = append(reasons, fmt.Sprintf("VM 内存不在 %dG~%dG 范围内: %dG", VMMemMin, VMMemMax, info.MemGB))
		}
	}

	return reasons
}

// resolvePaths determines data_path and install_path based on mount status and virtualization type.
func resolvePaths(hasData bool, virt string) (dataPath, installPath string) {
	if hasData && virt == "none" {
		return "/data", "/data"
	}
	return "/", "/"
}

// ResolvePaths performs a lightweight SSH mount check for hosts when --skip-check is used.
// It only checks /data mount status and sets resolved paths; hosts that fail SSH get "/" defaults.
func ResolvePaths(hosts []map[string]string, sshPort int, sshUser, sshPasswordB64 string, timeout time.Duration) []CheckResult {
	results := make([]CheckResult, 0, len(hosts))
	for _, host := range hosts {
		ip := strings.TrimSpace(host["server_ip"])
		if ip == "" {
			continue
		}
		cr := CheckResult{IP: ip, ResolvedDataPath: "/", ResolvedInstallPath: "/"}

		c, err := NewClient(ip, sshPort, sshUser, sshPasswordB64, timeout)
		if err != nil {
			// SSH failed — default to "/"
			results = append(results, cr)
			continue
		}

		_, err = c.Run("df -BG /data")
		c.Close()

		if err == nil {
			cr.ResolvedDataPath = "/data"
			cr.ResolvedInstallPath = "/data"
		}
		results = append(results, cr)
	}
	return results
}
