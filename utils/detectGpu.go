package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type GPUInfo struct {
	Vendor       string `json:"vendor"`
	Model        string `json:"model"`
	Memory       string `json:"memory,omitempty"`
	DriverVersion string `json:"driver_version,omitempty"`
	PCIAddress   string `json:"pci_address,omitempty"`
	RawOutput    string `json:"raw_output,omitempty"`
	Error        string `json:"error,omitempty"`
}

type GPUDetector struct {
	timeout time.Duration
}

func NewGPUDetector() *GPUDetector {
	return &GPUDetector{
		timeout: 10 * time.Second,
	}
}

func (d *GPUDetector) DetectGPUs() ([]GPUInfo, error) {
	switch runtime.GOOS {
	case "windows":
		return d.detectWindowsGPUs()
	case "linux":
		return d.detectLinuxGPUs()
	case "darwin":
		return d.detectMacGPUs()
	default:
		return []GPUInfo{{
			Vendor: "unknown",
			Model:  "unknown",
			Error:  fmt.Sprintf("Unsupported operating system: %s", runtime.GOOS),
		}}, nil
	}
}

// DetectGPUVendor maintains backward compatibility
func DetectGPUVendor() GPUInfo {
	detector := NewGPUDetector()
	gpus, err := detector.DetectGPUs()
	if err != nil || len(gpus) == 0 {
		return GPUInfo{
			Vendor: "unknown",
			Model:  "unknown",
			Error:  fmt.Sprintf("Detection failed: %v", err),
		}
	}
	return gpus[0] // Return primary GPU for backward compatibility
}

func (d *GPUDetector) runCommandWithTimeout(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func (d *GPUDetector) detectWindowsGPUs() ([]GPUInfo, error) {
	methods := []struct {
		desc string
		fn   func() ([]GPUInfo, error)
	}{
		{"PowerShell CIM", d.detectWindowsGPUsPowerShellCIM},
		{"PowerShell WMI", d.detectWindowsGPUsPowerShellWMI},
		{"WMIC", d.detectWindowsGPUsWMIC},
	}

	for _, method := range methods {
		if gpus, err := method.fn(); err == nil && len(gpus) > 0 {
			return gpus, nil
		}
	}

	return []GPUInfo{{
		Vendor: "unknown",
		Model:  "unknown",
		Error:  "Failed to detect GPU using all Windows methods",
	}}, nil
}

func (d *GPUDetector) detectWindowsGPUsPowerShellCIM() ([]GPUInfo, error) {
	cmd := `Get-CimInstance -ClassName Win32_VideoController | Where-Object {$_.Name -notlike '*Basic*' -and $_.Name -notlike '*Generic*' -and $_.Name -notlike '*VNC*'} | Select-Object Name, VideoProcessor, DriverVersion, AdapterRAM, PNPDeviceID | ConvertTo-Json`
	
	out, err := d.runCommandWithTimeout("powershell", "-NoProfile", "-Command", cmd)
	if err != nil {
		return nil, err
	}

	return d.parseWindowsGPUOutput(string(out)), nil
}

func (d *GPUDetector) detectWindowsGPUsPowerShellWMI() ([]GPUInfo, error) {
	cmd := `Get-WmiObject -Class Win32_VideoController | Where-Object {$_.Name -notlike '*Basic*' -and $_.Name -notlike '*Generic*'} | Select-Object Name, VideoProcessor, DriverVersion, AdapterRAM | Format-List`
	
	out, err := d.runCommandWithTimeout("powershell", "-NoProfile", "-Command", cmd)
	if err != nil {
		return nil, err
	}

	return d.parseWindowsGPUOutput(string(out)), nil
}

func (d *GPUDetector) detectWindowsGPUsWMIC() ([]GPUInfo, error) {
	out, err := d.runCommandWithTimeout("wmic", "path", "win32_VideoController", "get", "name,VideoProcessor,DriverVersion,AdapterRAM", "/format:list")
	if err != nil {
		return nil, err
	}

	return d.parseWindowsGPUOutput(string(out)), nil
}

func (d *GPUDetector) detectLinuxGPUs() ([]GPUInfo, error) {
	var gpus []GPUInfo

	// Try lspci first (most reliable)
	if lspciGPUs := d.tryLinuxLspci(); len(lspciGPUs) > 0 {
		gpus = append(gpus, lspciGPUs...)
	}

	// Try lshw for additional info
	if lshwGPUs := d.tryLinuxLshw(); len(lshwGPUs) > 0 {
		gpus = d.mergeLinuxGPUInfo(gpus, lshwGPUs)
	}

	// Check for NVIDIA via proc filesystem
	if nvidiaGPU := d.tryLinuxNvidiaProc(); nvidiaGPU.Vendor != "unknown" {
		gpus = d.mergeLinuxGPUInfo(gpus, []GPUInfo{nvidiaGPU})
	}

	// Try glxinfo for additional details
	if glxGPUs := d.tryLinuxGLX(); len(glxGPUs) > 0 {
		gpus = d.mergeLinuxGPUInfo(gpus, glxGPUs)
	}

	if len(gpus) == 0 {
		return []GPUInfo{{
			Vendor: "unknown",
			Model:  "unknown",
			Error:  "Could not detect GPU. Consider installing lspci, lshw, or mesa-utils",
		}}, nil
	}

	return gpus, nil
}

func (d *GPUDetector) tryLinuxLspci() []GPUInfo {
	out, err := d.runCommandWithTimeout("lspci", "-v", "-s", "$(lspci | grep -i vga | cut -d' ' -f1)")
	if err != nil {
		// Fallback to simple lspci
		if out, err = d.runCommandWithTimeout("lspci"); err != nil {
			return nil
		}
	}

	return d.parseLinuxLspciOutput(string(out))
}

func (d *GPUDetector) tryLinuxLshw() []GPUInfo {
	out, err := d.runCommandWithTimeout("lshw", "-C", "display")
	if err != nil {
		return nil
	}

	return d.parseLinuxLshwOutput(string(out))
}

func (d *GPUDetector) tryLinuxNvidiaProc() GPUInfo {
	if data, err := os.ReadFile("/proc/driver/nvidia/version"); err == nil {
		version := d.extractNvidiaVersionFromProc(string(data))
		return GPUInfo{
			Vendor:        "nvidia",
			Model:         "NVIDIA GPU",
			DriverVersion: version,
			RawOutput:     string(data),
		}
	}
	return GPUInfo{Vendor: "unknown"}
}

func (d *GPUDetector) tryLinuxGLX() []GPUInfo {
	out, err := d.runCommandWithTimeout("glxinfo")
	if err != nil {
		return nil
	}

	return d.parseLinuxGLXOutput(string(out))
}

func (d *GPUDetector) detectMacGPUs() ([]GPUInfo, error) {
	out, err := d.runCommandWithTimeout("system_profiler", "SPDisplaysDataType")
	if err != nil {
		return []GPUInfo{{
			Vendor: "unknown",
			Model:  "unknown",
			Error:  fmt.Sprintf("Failed to run system_profiler: %v", err),
		}}, nil
	}

	return d.parseMacGPUOutput(string(out)), nil
}

// Enhanced parsing functions

func (d *GPUDetector) parseWindowsGPUOutput(output string) []GPUInfo {
	var gpus []GPUInfo
	
	// Handle JSON output from CIM
	if strings.Contains(output, "{") && strings.Contains(output, "}") {
		return d.parseWindowsJSONOutput(output)
	}

	// Parse traditional text output
	blocks := d.splitIntoBlocks(output)
	for _, block := range blocks {
		gpu := GPUInfo{RawOutput: block}
		
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				key := strings.TrimSpace(strings.ToLower(parts[0]))
				value := strings.TrimSpace(parts[1])

				switch {
				case strings.Contains(key, "name"):
					gpu.Model = value
				case strings.Contains(key, "videoprocessor"):
					if gpu.Model == "" {
						gpu.Model = value
					}
				case strings.Contains(key, "driverversion"):
					gpu.DriverVersion = value
				case strings.Contains(key, "adapterram"):
					gpu.Memory = d.formatMemory(value)
				}
			}
		}

		if gpu.Model != "" && !d.isGenericGPU(gpu.Model) {
			gpu.Vendor = d.determineVendorFromOutput(gpu.Model)
			gpus = append(gpus, gpu)
		}
	}

	return gpus
}

func (d *GPUDetector) parseLinuxLspciOutput(output string) []GPUInfo {
	var gpus []GPUInfo
	
	re := regexp.MustCompile(`(?i)(VGA|3D|Display).*?:\s*(.+)`)
	matches := re.FindAllStringSubmatch(output, -1)
	
	for _, match := range matches {
		if len(match) > 2 {
			model := strings.TrimSpace(match[2])
			if !d.isGenericGPU(model) {
				gpu := GPUInfo{
					Vendor:    d.determineVendorFromOutput(model),
					Model:     model,
					RawOutput: output,
				}
				
				// Extract PCI address
				if pciAddr := d.extractPCIAddress(output, model); pciAddr != "" {
					gpu.PCIAddress = pciAddr
				}
				
				gpus = append(gpus, gpu)
			}
		}
	}
	
	return gpus
}

func (d *GPUDetector) parseLinuxLshwOutput(output string) []GPUInfo {
	var gpus []GPUInfo
	blocks := d.splitIntoBlocks(output)
	
	for _, block := range blocks {
		gpu := GPUInfo{RawOutput: block}
		
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "product:") {
				gpu.Model = strings.TrimSpace(strings.TrimPrefix(line, "product:"))
			} else if strings.HasPrefix(line, "vendor:") {
				vendor := strings.TrimSpace(strings.TrimPrefix(line, "vendor:"))
				if gpu.Vendor == "" || gpu.Vendor == "unknown" {
					gpu.Vendor = d.normalizeVendorName(vendor)
				}
			} else if strings.Contains(line, "size:") && strings.Contains(line, "bytes") {
				gpu.Memory = d.extractMemoryFromSize(line)
			}
		}
		
		if gpu.Model != "" && !d.isGenericGPU(gpu.Model) {
			if gpu.Vendor == "" || gpu.Vendor == "unknown" {
				gpu.Vendor = d.determineVendorFromOutput(gpu.Model)
			}
			gpus = append(gpus, gpu)
		}
	}
	
	return gpus
}

func (d *GPUDetector) parseMacGPUOutput(output string) []GPUInfo {
	var gpus []GPUInfo
	blocks := d.splitIntoBlocks(output)
	
	for _, block := range blocks {
		gpu := GPUInfo{RawOutput: block}
		
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "Chipset Model:") {
				if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
					gpu.Model = strings.TrimSpace(parts[1])
				}
			} else if strings.Contains(line, "VRAM") && strings.Contains(line, ":") {
				if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
					gpu.Memory = strings.TrimSpace(parts[1])
				}
			}
		}
		
		if gpu.Model != "" {
			gpu.Vendor = d.determineVendorFromOutput(gpu.Model)
			gpus = append(gpus, gpu)
		}
	}
	
	return gpus
}

// Enhanced vendor detection
func (d *GPUDetector) determineVendorFromOutput(output string) string {
	lower := strings.ToLower(output)
	
	// NVIDIA patterns (most specific first)
	nvidiaPatterns := []string{
		"nvidia", "geforce", "quadro", "tesla", "rtx", "gtx", "titan", "nvs",
	}
	for _, pattern := range nvidiaPatterns {
		if strings.Contains(lower, pattern) {
			return "nvidia"
		}
	}
	
	// AMD patterns
	amdPatterns := []string{
		"amd", "radeon", "rx ", "vega", "navi", "rdna", "ati", "firepro", "firegl",
	}
	for _, pattern := range amdPatterns {
		if strings.Contains(lower, pattern) {
			return "amd"
		}
	}
	
	// Intel patterns
	intelPatterns := []string{
		"intel", "iris", "uhd graphics", "hd graphics", "xe graphics", "arc",
	}
	for _, pattern := range intelPatterns {
		if strings.Contains(lower, pattern) {
			return "intel"
		}
	}
	
	// Apple patterns
	applePatterns := []string{
		"apple", "m1", "m2", "m3", "m4",
	}
	for _, pattern := range applePatterns {
		if strings.Contains(lower, pattern) {
			return "apple"
		}
	}
	
	return "unknown"
}

// Helper functions
func (d *GPUDetector) splitIntoBlocks(output string) []string {
	var blocks []string
	var currentBlock strings.Builder
	
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" && currentBlock.Len() > 0 {
			blocks = append(blocks, currentBlock.String())
			currentBlock.Reset()
		} else if strings.TrimSpace(line) != "" {
			if currentBlock.Len() > 0 {
				currentBlock.WriteString("\n")
			}
			currentBlock.WriteString(line)
		}
	}
	
	if currentBlock.Len() > 0 {
		blocks = append(blocks, currentBlock.String())
	}
	
	return blocks
}

func (d *GPUDetector) isGenericGPU(model string) bool {
	genericTerms := []string{
		"basic", "generic", "standard", "vnc", "virtual", "vmware", "vbox",
	}
	
	lower := strings.ToLower(model)
	for _, term := range genericTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func (d *GPUDetector) formatMemory(memStr string) string {
	// Extract numeric value and convert bytes to more readable format
	re := regexp.MustCompile(`\d+`)
	if match := re.FindString(memStr); match != "" {
		if bytes, err := strconv.ParseInt(match, 10, 64); err == nil {
			if bytes >= 1024*1024*1024 {
				return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
			} else if bytes >= 1024*1024 {
				return fmt.Sprintf("%.0f MB", float64(bytes)/(1024*1024))
			}
		}
	}
	return memStr
}

func (d *GPUDetector) normalizeVendorName(vendor string) string {
	lower := strings.ToLower(vendor)
	switch {
	case strings.Contains(lower, "nvidia"):
		return "nvidia"
	case strings.Contains(lower, "amd") || strings.Contains(lower, "ati"):
		return "amd"
	case strings.Contains(lower, "intel"):
		return "intel"
	case strings.Contains(lower, "apple"):
		return "apple"
	default:
		return strings.ToLower(vendor)
	}
}

func (d *GPUDetector) extractPCIAddress(output, model string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.Contains(line, model) && i > 0 {
			prevLine := lines[i-1]
			if re := regexp.MustCompile(`^([0-9a-f]{2}:[0-9a-f]{2}\.[0-9a-f])`); re.MatchString(prevLine) {
				return re.FindString(prevLine)
			}
		}
	}
	return ""
}

func (d *GPUDetector) extractNvidiaVersionFromProc(data string) string {
	re := regexp.MustCompile(`NVIDIA.*?(\d+\.\d+(?:\.\d+)?)`)
	if match := re.FindStringSubmatch(data); len(match) > 1 {
		return match[1]
	}
	return ""
}

func (d *GPUDetector) extractMemoryFromSize(line string) string {
	re := regexp.MustCompile(`size:\s*(\d+)\s*bytes`)
	if match := re.FindStringSubmatch(line); len(match) > 1 {
		if _, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			return d.formatMemory(match[1])
		}
	}
	return ""
}

func (d *GPUDetector) parseWindowsJSONOutput(output string) []GPUInfo {
	// Basic JSON parsing - in production, use encoding/json
	var gpus []GPUInfo
	
	// This is a simplified parser - you'd want to use proper JSON unmarshaling
	blocks := strings.Split(output, "},{")
	for _, block := range blocks {
		gpu := GPUInfo{}
		
		if nameMatch := regexp.MustCompile(`"Name":\s*"([^"]+)"`).FindStringSubmatch(block); len(nameMatch) > 1 {
			gpu.Model = nameMatch[1]
		}
		
		if driverMatch := regexp.MustCompile(`"DriverVersion":\s*"([^"]+)"`).FindStringSubmatch(block); len(driverMatch) > 1 {
			gpu.DriverVersion = driverMatch[1]
		}
		
		if ramMatch := regexp.MustCompile(`"AdapterRAM":\s*(\d+)`).FindStringSubmatch(block); len(ramMatch) > 1 {
			gpu.Memory = d.formatMemory(ramMatch[1])
		}
		
		if gpu.Model != "" && !d.isGenericGPU(gpu.Model) {
			gpu.Vendor = d.determineVendorFromOutput(gpu.Model)
			gpu.RawOutput = block
			gpus = append(gpus, gpu)
		}
	}
	
	return gpus
}

func (d *GPUDetector) parseLinuxGLXOutput(output string) []GPUInfo {
	var gpus []GPUInfo
	
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "OpenGL renderer string:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				model := strings.TrimSpace(parts[1])
				if !d.isGenericGPU(model) {
					gpu := GPUInfo{
						Vendor:    d.determineVendorFromOutput(model),
						Model:     model,
						RawOutput: output,
					}
					gpus = append(gpus, gpu)
				}
			}
		}
	}
	
	return gpus
}

func (d *GPUDetector) mergeLinuxGPUInfo(existing []GPUInfo, new []GPUInfo) []GPUInfo {
	// Simple merge strategy - in production, you'd want more sophisticated matching
	result := make([]GPUInfo, len(existing))
	copy(result, existing)
	
	for _, newGPU := range new {
		found := false
		for i, existingGPU := range result {
			if d.areGPUsSimilar(existingGPU, newGPU) {
				// Merge information
				if result[i].Memory == "" && newGPU.Memory != "" {
					result[i].Memory = newGPU.Memory
				}
				if result[i].DriverVersion == "" && newGPU.DriverVersion != "" {
					result[i].DriverVersion = newGPU.DriverVersion
				}
				if result[i].PCIAddress == "" && newGPU.PCIAddress != "" {
					result[i].PCIAddress = newGPU.PCIAddress
				}
				found = true
				break
			}
		}
		if !found {
			result = append(result, newGPU)
		}
	}
	
	return result
}

func (d *GPUDetector) areGPUsSimilar(gpu1, gpu2 GPUInfo) bool {
	return gpu1.Vendor == gpu2.Vendor && 
		   (strings.Contains(gpu1.Model, gpu2.Model) || strings.Contains(gpu2.Model, gpu1.Model))
}