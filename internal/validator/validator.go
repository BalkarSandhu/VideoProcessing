package validator

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"video_processing/internal/config"
)

// Validator handles system validation
type Validator struct{}

// New creates a new validator instance
func New() *Validator {
	return &Validator{}
}

// ValidateSetup validates the system setup for video processing
func (v *Validator) ValidateSetup(config *config.ProcessingConfig) error {
	// Check FFmpeg availability
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH. Please install FFmpeg")
	}

	// VAAPI-specific checks
	if config.Acceleration == "vaapi" {
		return v.validateVAAPISetup()
	}

	return nil
}

func (v *Validator) validateVAAPISetup() error {
	fmt.Println("🔧 Validating VAAPI setup...")

	// Check render nodes
	renderNodes := []string{"/dev/dri/renderD128", "/dev/dri/renderD129"}
	var foundNode bool

	for _, node := range renderNodes {
		if _, err := os.Stat(node); err == nil {
			fmt.Printf("✅ Found render node: %s\n", node)
			foundNode = true
			break
		}
	}

	if !foundNode {
		fmt.Println("⚠️  No render nodes found. VAAPI may not work properly.")
		fmt.Println("   Install drivers: sudo apt install mesa-va-drivers intel-media-va-driver")
		fmt.Println("   Add to video group: sudo usermod -a -G video $USER")
	}

	// Check vainfo if available
	if _, err := exec.LookPath("vainfo"); err == nil {
		cmd := exec.Command("vainfo", "-a")
		if out, err := cmd.Output(); err == nil {
			output := string(out)
			if strings.Contains(strings.ToLower(output), "h264") {
				fmt.Println("✅ VAAPI H.264 encoding support detected")
			} else {
				fmt.Println("⚠️  H.264 encoding may not be available")
			}
		}
	}

	return nil
}
