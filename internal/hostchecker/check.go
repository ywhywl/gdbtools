package hostchecker

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// CheckAll checks all hosts via SSH and returns results.
func CheckAll(hosts []map[string]string, sshPort int, sshUser string, auth *SSHAuth, timeout time.Duration, requiredOSVersion string) []CheckResult {
	results := make([]CheckResult, 0, len(hosts))
	for _, host := range hosts {
		ip := strings.TrimSpace(host["server_ip"])
		if ip == "" {
			continue
		}
		log.Printf("[check] 检查主机: %s", ip)

		result, err := checkHost(ip, sshPort, sshUser, auth, timeout, requiredOSVersion)
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
		log.Printf("[check] %s %s: os=%s/%s cpu_vendor=%s cpu_model=%s arch=%s virt=%s cpu=%d mem=%dG data_mount=%v data_avail=%dG",
			status, ip, result.SysInfo.OS, result.SysInfo.OSVersion,
			result.SysInfo.CPUVendor, result.SysInfo.CPUModel, result.SysInfo.CPUArch,
			result.SysInfo.Virt,
			result.SysInfo.CPU, result.SysInfo.MemGB, result.SysInfo.HasData, result.SysInfo.DataAvailGB)
		if !result.Passed && len(result.Reasons) > 0 {
			for _, reason := range result.Reasons {
				log.Printf("[check]   原因: %s", reason)
			}
		}
	}
	return results
}

func checkHost(ip string, sshPort int, sshUser string, auth *SSHAuth, timeout time.Duration, requiredOSVersion string) (CheckResult, error) {
	c, err := NewClient(ip, sshPort, sshUser, auth, timeout)
	if err != nil {
		return CheckResult{IP: ip, SysInfo: &SysInfo{IP: ip}}, err
	}
	defer c.Close()

	sysInfo, err := collectSysInfo(c, ip)
	if err != nil {
		return CheckResult{IP: ip, SysInfo: &sysInfo}, err
	}

	reasons := evaluateRulesWithConfig(sysInfo, requiredOSVersion)

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

	// 4. /data mount check - verify it's a real mount point, not just a directory
	out, err = c.Run("df -BG /data | awk 'NR==2{print $6}'")
	info.HasData = false
	if err == nil {
		mountPoint := strings.TrimSpace(out)
		info.HasData = (mountPoint == "/data")
	}
	if info.HasData {
		// get available space
		out2, err2 := c.Run("df -BG /data | awk 'NR==2{gsub(/G/,\"\",$4); print $4}'")
		if err2 == nil {
			if val, parseErr := strconv.Atoi(out2); parseErr == nil {
				info.DataAvailGB = val
			}
		}
	}

	// 5. OS detection with version
	osID, osVersion := detectOSWithVersion(c)
	info.OS = osID
	info.OSVersion = osVersion

	// 6. CPU model and vendor detection
	cpuModel, cpuVendor := detectCPU(c)
	info.CPUModel = cpuModel
	info.CPUVendor = cpuVendor

	// 7. Virtualization detection (DMI + CPU hypervisor flag)
	info.Virt = c.DetectVirt()

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

func detectOSWithVersion(c *Client) (os, version string) {
	// Try /etc/os-release first
	osID, _ := c.Run("grep -oP '(?<=^ID=).+' /etc/os-release | tr -d '\"'")
	osVersion, _ := c.Run("grep -oP '(?<=^VERSION_ID=).+' /etc/os-release | tr -d '\"'")

	osID = strings.TrimSpace(strings.ToLower(osID))
	osVersion = strings.TrimSpace(osVersion)

	// Fallback: check release files if os-release failed
	if osID == "" {
		osID = detectOSFromReleaseFiles(c)
	}

	// Normalize OS
	normalizedOS := normalizeOS(osID)

	// Extract version (for Kylin, version might be "V10", "10", "V10 (Sword)", etc.)
	if normalizedOS == OSKylin && osVersion != "" {
		// Handle formats: "V10", "10", "V10 SP1", "V10 (Sword)"
		upper := strings.ToUpper(osVersion)
		if strings.HasPrefix(upper, "V") {
			// Extract "V10" from "V10 (Sword)" or "V10 SP1"
			parts := strings.Fields(upper)
			if len(parts) > 0 {
				return normalizedOS, parts[0] // "V10"
			}
		} else if osVersion == "10" {
			return normalizedOS, "V10"
		}
		return normalizedOS, upper
	}

	return normalizedOS, osVersion
}

func detectCPU(c *Client) (model, vendor string) {
	// Try lscpu first (works for both x86 and ARM)
	out, err := c.Run("lscpu | grep -i '^Model name:' | cut -d: -f2")
	if err == nil && strings.TrimSpace(out) != "" {
		model = strings.TrimSpace(out)
		vendor = normalizeCPUVendor(model)
		return
	}

	// Fallback: x86_64 /proc/cpuinfo
	out, err = c.Run("grep -m1 '^model name' /proc/cpuinfo | cut -d: -f2")
	if err == nil && strings.TrimSpace(out) != "" {
		model = strings.TrimSpace(out)
		vendor = normalizeCPUVendor(model)
		return
	}

	// Fallback: aarch64 - try CPU implementer + CPU part
	impl, err1 := c.Run("grep -m1 '^CPU implementer' /proc/cpuinfo | awk '{print $NF}'")
	part, err2 := c.Run("grep -m1 '^CPU part' /proc/cpuinfo | awk '{print $NF}'")

	if err1 == nil && err2 == nil {
		impl = strings.TrimSpace(impl)
		part = strings.TrimSpace(part)

		// Kunpeng: implementer 0x48 (Hisilicon)
		if impl == "0x48" || impl == "0x0048" {
			model = fmt.Sprintf("Kunpeng (implementer: %s, part: %s)", impl, part)
			vendor = CPUVendorKunpeng
			return
		}

		model = fmt.Sprintf("ARM CPU (implementer: %s, part: %s)", impl, part)
		vendor = CPUVendorUnknown
		return
	}

	return "unknown", CPUVendorUnknown
}

func normalizeCPUVendor(model string) string {
	lower := strings.ToLower(model)

	// 海光 Hygon
	if strings.Contains(lower, "hygon") {
		return CPUVendorHygon
	}

	// 鲲鹏 Kunpeng / Hisilicon
	if strings.Contains(lower, "kunpeng") || strings.Contains(lower, "hisilicon") {
		return CPUVendorKunpeng
	}

	// Intel
	if strings.Contains(lower, "intel") {
		return CPUVendorIntel
	}

	// AMD (but not Hygon, which is AMD-derived)
	if strings.Contains(lower, "amd") {
		return CPUVendorAMD
	}

	return CPUVendorUnknown
}

func isHygon71Series(model string) bool {
	// Match Hygon C86 71xx series (7100-7199)
	// Examples: "Hygon C86 7151", "Hygon C86 7171"
	lower := strings.ToLower(model)
	if !strings.Contains(lower, "hygon") {
		return false
	}

	// Extract numbers from model string
	parts := strings.Fields(model)
	for _, part := range parts {
		// Check if it's a 4-digit number starting with 71
		if len(part) == 4 {
			if strings.HasPrefix(part, "71") {
				// Verify all characters are digits
				if _, err := strconv.Atoi(part); err == nil {
					return true
				}
			}
		}
	}

	return false
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
	return evaluateRulesWithConfig(info, "V10")
}

func evaluateRulesWithConfig(info SysInfo, requiredOSVersion string) []string {
	var reasons []string

	// 1. OS version check — must be Kylin with specified version
	if info.OS != OSKylin {
		if info.OS == OSCentOS {
			reasons = append(reasons, fmt.Sprintf("操作系统为 CentOS，要求麒麟 %s", requiredOSVersion))
		} else {
			reasons = append(reasons, fmt.Sprintf("操作系统为 %s，要求麒麟 %s", info.OS, requiredOSVersion))
		}
	} else if info.OSVersion != requiredOSVersion {
		reasons = append(reasons, fmt.Sprintf("操作系统版本为麒麟 %s，要求麒麟 %s", info.OSVersion, requiredOSVersion))
	}

	// 2. CPU vendor check — must be Hygon (non-71xx) or Kunpeng
	switch info.CPUVendor {
	case CPUVendorHygon:
		// Check if it's disallowed 71xx series
		if isHygon71Series(info.CPUModel) {
			reasons = append(reasons, fmt.Sprintf("海光 CPU 71xx 系列不支持: %s", info.CPUModel))
		}
	case CPUVendorKunpeng:
		// Kunpeng is always allowed
	case CPUVendorIntel:
		reasons = append(reasons, fmt.Sprintf("CPU 类型不支持: 检测到 Intel CPU (%s)，仅支持海光（非71xx系列）或鲲鹏", info.CPUModel))
	case CPUVendorAMD:
		reasons = append(reasons, fmt.Sprintf("CPU 类型不支持: 检测到 AMD CPU (%s)，仅支持海光（非71xx系列）或鲲鹏", info.CPUModel))
	default:
		reasons = append(reasons, fmt.Sprintf("CPU 类型未知或不支持: %s，仅支持海光（非71xx系列）或鲲鹏", info.CPUModel))
	}

	if info.Virt == "none" {
		// Physical machine rules
		physMemThreshold := PhysMemMin - MemToleranceGB
		if !info.HasData {
			reasons = append(reasons, "/data 目录未挂载")
		} else if info.DataAvailGB < PhysDataAvailMin-DataToleranceGB {
			reasons = append(reasons, fmt.Sprintf("/data 可用空间不足: %dG < %dG (%dG - %dG 容差)",
				info.DataAvailGB, PhysDataAvailMin-DataToleranceGB, PhysDataAvailMin, DataToleranceGB))
		}
		if info.CPU < PhysCPUMin {
			reasons = append(reasons, fmt.Sprintf("CPU 核心数不足: %d < %d", info.CPU, PhysCPUMin))
		}
		if info.MemGB < physMemThreshold {
			reasons = append(reasons, fmt.Sprintf("内存不足: %dG < %dG (%dG - %dG 容差)",
				info.MemGB, physMemThreshold, PhysMemMin, MemToleranceGB))
		}
	} else {
		// VM rules
		vmMemUpper := VMMemMax + MemToleranceGB
		if info.HasData {
			reasons = append(reasons, "虚拟机不应挂载 /data 目录")
		}
		if info.CPU > VMCPUMax {
			reasons = append(reasons, fmt.Sprintf("VM CPU 核心数过多: %d > %d", info.CPU, VMCPUMax))
		}
		if info.MemGB < VMMemMin || info.MemGB > vmMemUpper {
			reasons = append(reasons, fmt.Sprintf("VM 内存不在 %dG~%dG 范围内: %dG", VMMemMin, vmMemUpper, info.MemGB))
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
func ResolvePaths(hosts []map[string]string, sshPort int, sshUser string, auth *SSHAuth, timeout time.Duration) []CheckResult {
	results := make([]CheckResult, 0, len(hosts))
	for _, host := range hosts {
		ip := strings.TrimSpace(host["server_ip"])
		if ip == "" {
			continue
		}
		cr := CheckResult{IP: ip, ResolvedDataPath: "/", ResolvedInstallPath: "/"}

		c, err := NewClient(ip, sshPort, sshUser, auth, timeout)
		if err != nil {
			log.Printf("[path-resolve] FAIL %s: SSH 连接失败: %s", ip, err)
			results = append(results, cr)
			continue
		}

		out, err := c.Run("df -BG /data | awk 'NR==2{print $6}'")
		c.Close()

		if err != nil {
			log.Printf("[path-resolve] WARN %s: df /data 失败: %s, 使用默认路径 /", ip, err)
		} else {
			mountPoint := strings.TrimSpace(out)
			if mountPoint == "/data" {
				cr.ResolvedDataPath = "/data"
				cr.ResolvedInstallPath = "/data"
				log.Printf("[path-resolve] INFO %s: 检测到独立 /data 挂载点", ip)
			} else {
				log.Printf("[path-resolve] INFO %s: /data 不是独立挂载点 (当前挂载: %s), 使用默认路径 /", ip, mountPoint)
			}
		}
		results = append(results, cr)
	}
	return results
}
