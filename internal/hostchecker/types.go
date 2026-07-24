package hostchecker

// SysInfo holds system information collected from a host via SSH.
type SysInfo struct {
	IP          string
	Virt        string // "none" = physical, otherwise VM type
	CPU         int    // CPU core count
	MemGB       int    // memory in GB
	HasData     bool   // whether /data is mounted
	DataAvailGB int    // /data available space in GB
	OS          string // "kylin", "centos", or other
	OSVersion   string // OS version, e.g. "V10", "V10 SP1"
	CPUArch     string // "aarch64", "x86_64", etc.
	CPUVendor   string // "hygon", "kunpeng", "intel", "amd", "unknown"
	CPUModel    string // full CPU model string from lscpu or /proc/cpuinfo
}

// CheckResult holds the result of checking a single host.
type CheckResult struct {
	IP                  string
	Passed              bool
	Reasons             []string // failure reasons (empty if passed)
	SysInfo             *SysInfo
	ResolvedDataPath    string // auto-resolved data_path based on mount check
	ResolvedInstallPath string // auto-resolved install_path based on mount check
}

// Rule thresholds
const (
	// Physical machine requirements
	PhysDataAvailMin = 3072 // 3T in GB
	PhysCPUMin       = 50
	PhysMemMin       = 200

	// VM requirements
	VMCPUMax = 30
	VMMemMin = 24
	VMMemMax = 48

	// Tolerance for measurement偏差
	MemToleranceGB  = 4  // free -g 向下取整误差
	DataToleranceGB = 64 // df -BG 向下取整误差

	// Supported OS IDs
	OSKylin    = "kylin"
	OSCentOS   = "centos"
	OSNeoKylin = "neokylin"

	// CPU vendor types
	CPUVendorHygon   = "hygon"   // 海光
	CPUVendorKunpeng = "kunpeng" // 鲲鹏
	CPUVendorIntel   = "intel"
	CPUVendorAMD     = "amd"
	CPUVendorUnknown = "unknown"
)
