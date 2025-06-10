package processor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"video_processing/internal/config"
	"video_processing/internal/encoder"
	"video_processing/internal/player"
	"video_processing/internal/validator"
	"video_processing/utils"
)

// Processor is the main video processor
type Processor struct {
	gpuDetector     *utils.GPUDetector
	encoder         *encoder.Encoder
	commandBuilder  *encoder.CommandBuilder
	fallbackManager *encoder.FallbackManager
	validator       *validator.Validator
	player          *player.Player
	reader          *bufio.Reader
}

// New creates a new processor instance
func New() *Processor {
	return &Processor{
		gpuDetector:     utils.NewGPUDetector(),
		encoder:         encoder.New(),
		commandBuilder:  encoder.NewCommandBuilder(),
		fallbackManager: encoder.NewFallbackManager(),
		validator:       validator.New(),
		player:          player.New(),
		reader:          bufio.NewReader(os.Stdin),
	}
}

// Run executes the complete video processing workflow
func (p *Processor) Run() error {
	fmt.Println("🎬 FFmpeg GPU-Accelerated Video Processor")
	fmt.Println(strings.Repeat("=", 50))

	// Step 1: Detect GPUs
	gpus, err := p.detectAndDisplayGPUs()
	if err != nil {
		return fmt.Errorf("GPU detection failed: %w", err)
	}

	// Step 2: Configure processing based on detected hardware
	config, err := p.configureProcessing(gpus)
	if err != nil {
		return fmt.Errorf("configuration failed: %w", err)
	}

	// Step 3: Validate setup
	if err := p.validator.ValidateSetup(config); err != nil {
		fmt.Printf("⚠️  Setup validation warnings: %v\n", err)
	}

	// Step 4: Get user input
	if err := p.getUserInput(config); err != nil {
		return fmt.Errorf("input failed: %w", err)
	}

	// Step 5: Process video
	if err := p.processVideo(config); err != nil {
		return fmt.Errorf("video processing failed: %w", err)
	}

	// Step 6: Optional playback
	return p.player.OfferPlayback(config.OutputPath)
}

func (p *Processor) detectAndDisplayGPUs() ([]utils.GPUInfo, error) {
	fmt.Println("🔍 Detecting GPU hardware...")

	gpus, err := p.gpuDetector.DetectGPUs()
	if err != nil {
		return nil, err
	}

	if len(gpus) == 0 {
		fmt.Println("❌ No GPUs detected")
		return gpus, nil
	}

	fmt.Printf("✅ Found %d GPU(s):\n", len(gpus))
	for i, gpu := range gpus {
		fmt.Printf("  %d. %s %s", i+1, strings.Title(gpu.Vendor), gpu.Model)
		if gpu.Memory != "" {
			fmt.Printf(" (%s)", gpu.Memory)
		}
		if gpu.DriverVersion != "" {
			fmt.Printf(" [Driver: %s]", gpu.DriverVersion)
		}
		fmt.Println()

		if gpu.Error != "" {
			fmt.Printf("     ⚠️  Warning: %s\n", gpu.Error)
		}
	}

	fmt.Println(strings.Repeat("-", 50))
	return gpus, nil
}

func (p *Processor) configureProcessing(gpus []utils.GPUInfo) (*config.ProcessingConfig, error) {
	cfg := config.NewDefault()

	if len(gpus) == 0 || gpus[0].Vendor == "unknown" {
		fmt.Println("🔄 Using software encoding (no GPU acceleration)")
		cfg.SetSoftwareEncoding()
		return cfg, nil
	}

	// Use the primary (first) GPU
	primaryGPU := gpus[0]
	acceleration, codec, preset := p.encoder.ConfigureForGPU(primaryGPU)
	cfg.SetHardwareEncoding(acceleration, codec, preset)

	fmt.Printf("🚀 Hardware acceleration: %s (%s)\n", cfg.Acceleration, cfg.Codec)
	fmt.Printf("📊 Quality setting: %d, Preset: %s\n", cfg.Quality, cfg.Preset)
	fmt.Println(strings.Repeat("-", 50))

	return cfg, nil
}

func (p *Processor) getUserInput(cfg *config.ProcessingConfig) error {
	// Get input file/URL
	fmt.Print("📁 Enter input video file path or stream URL: ")
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return err
	}

	cfg.InputPath = strings.TrimSpace(input)
	if cfg.InputPath == "" {
		return fmt.Errorf("no input provided")
	}

	// Optional: Get output path
	fmt.Printf("💾 Output file (default: %s): ", cfg.OutputPath)
	output, _ := p.reader.ReadString('\n')
	output = strings.TrimSpace(output)
	if output != "" {
		cfg.OutputPath = output
	}

	// Optional: Quality setting
	fmt.Printf("🎚️  Quality (CRF/QP, default: %d, lower=better): ", cfg.Quality)
	qualityStr, _ := p.reader.ReadString('\n')
	qualityStr = strings.TrimSpace(qualityStr)
	if qualityStr != "" {
		var quality int
		if _, err := fmt.Sscanf(qualityStr, "%d", &quality); err == nil && quality >= 0 && quality <= 51 {
			cfg.Quality = quality
		}
	}

	return nil
}

func (p *Processor) processVideo(cfg *config.ProcessingConfig) error {
	fmt.Println("\n🎬 Starting video processing...")

	args := p.commandBuilder.BuildFFmpegCommand(cfg)
	fmt.Printf("Command: ffmpeg %s\n", strings.Join(args, " "))
	fmt.Println(strings.Repeat("-", 50))

	// Use context with timeout (optional, can use cancel context too)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup command
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = os.Stderr // FFmpeg logs (progress, errors)
	cmd.Stdout = os.Stdout // Optional: capture output if needed

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ FFmpeg exited with error: %v\n", err)

		// Log detailed FFmpeg error output
		if stderr.Len() > 0 {
			fmt.Println("🔍 FFmpeg stderr output:")
			fmt.Println(stderr.String())
		}

		// Check if context was cancelled (e.g., timeout, manual cancel)
		if ctx.Err() != nil {
			fmt.Printf("⚠️  Command was cancelled: %v\n", ctx.Err())
		}

		// Try fallbacks
		if fallbackErr := p.fallbackManager.TryFallbacks(cfg); fallbackErr != nil {
			return fmt.Errorf("all encoding methods failed: %w", fallbackErr)
		}
	}

	fmt.Printf("✅ Video processing completed in %v\n", duration.Round(time.Second))
	fmt.Printf("📁 Output saved to: %s\n", cfg.OutputPath)

	if info, err := os.Stat(cfg.OutputPath); err == nil {
		fmt.Printf("📊 Output file size: %.2f MB\n", float64(info.Size())/(1024*1024))
	}

	return nil
}
