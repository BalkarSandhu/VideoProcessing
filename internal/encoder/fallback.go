package encoder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"video_processing/internal/config"
)

// FallbackMethod represents a fallback encoding method
type FallbackMethod struct {
	Description string
	Args        []string
}

// FallbackManager handles fallback encoding strategies
type FallbackManager struct{}

// NewFallbackManager creates a new fallback manager
func NewFallbackManager() *FallbackManager {
	return &FallbackManager{}
}

// TryFallbacks attempts fallback encoding methods with live FFmpeg logs
func (fm *FallbackManager) TryFallbacks(config *config.ProcessingConfig) error {
	fallbacks := fm.getFallbackMethods(config)

	for i, fallback := range fallbacks {
		fmt.Printf("\n🔁 Attempt %d/%d: %s\n", i+1, len(fallbacks), fallback.Description)
		fmt.Printf("▶️ Running: ffmpeg %s\n", formatArgsForDisplay(fallback.Args))

		cmd := exec.Command("ffmpeg", fallback.Args...)
		cmd.Stderr = os.Stderr // FFmpeg logs (progress, errors)
		cmd.Stdout = os.Stdout // Optional: capture output if needed

		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Fallback %d failed: %v\n", i+1, err)
			continue
		}

		fmt.Printf("✅ Fallback method succeeded: %s\n", fallback.Description)
		return nil
	}

	return fmt.Errorf("❌ All fallback encoding methods failed")
}

// getFallbackMethods returns a list of fallback encoding strategies
func (fm *FallbackManager) getFallbackMethods(config *config.ProcessingConfig) []FallbackMethod {
	// Build base arguments for software encoding
	baseArgs := []string{
		"-i", config.InputPath,
		"-c:v", "libx264",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-fflags", "+discardcorrupt",
		"-analyzeduration", "0",
		"-probesize", "32",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-crf", fmt.Sprintf("%d", config.Quality),
		"-c:a", "copy",
	}

	// Add output format based on output path
	argsWithFormat := fm.addOutputFormat(baseArgs, config.OutputPath)

	// Add final output options
	finalArgs := append(argsWithFormat,
		"-movflags", "+faststart",
		"-y", config.OutputPath,
	)

	return []FallbackMethod{
		{
			Description: "Software encoding (libx264) with auto-detected output format",
			Args:        finalArgs,
		},
		{
			Description: "Software encoding (libx264) with MP4 format fallback",
			Args: append(fm.addMP4Fallback(baseArgs),
				"-movflags", "+faststart",
				"-y", config.OutputPath,
			),
		},
		{
			Description: "Basic software encoding (minimal options)",
			Args: []string{
				"-i", config.InputPath,
				"-c:v", "libx264",
				"-preset", "ultrafast",
				"-crf", fmt.Sprintf("%d", config.Quality),
				"-c:a", "copy",
				"-y", config.OutputPath,
			},
		},
	}
}

// addOutputFormat adds the appropriate output format based on the output path/URL
func (fm *FallbackManager) addOutputFormat(args []string, outputPath string) []string {
	// Check if it's a streaming URL
	if fm.isStreamingURL(outputPath) {
		return fm.addStreamingFormat(args, outputPath)
	}

	// For file outputs, determine format from extension
	ext := strings.ToLower(filepath.Ext(outputPath))
	switch ext {
	case ".mp4":
		args = append(args, "-f", "mp4")
	case ".mkv":
		args = append(args, "-f", "matroska")
	case ".avi":
		args = append(args, "-f", "avi")
	case ".mov":
		args = append(args, "-f", "mov")
	case ".webm":
		args = append(args, "-f", "webm")
	case ".flv":
		args = append(args, "-f", "flv")
	case ".ts":
		args = append(args, "-f", "mpegts")
	case ".m3u8":
		args = append(args, "-f", "hls")
		args = append(args, "-hls_time", "10")
		args = append(args, "-hls_list_size", "0")
	default:
		// Default to mp4 if extension is unknown or missing
		args = append(args, "-f", "mp4")
	}

	return args
}

// addMP4Fallback adds MP4 format as a safe fallback
func (fm *FallbackManager) addMP4Fallback(args []string) []string {
	return append(args, "-f", "mp4")
}

// isStreamingURL checks if the output path is a streaming URL
func (fm *FallbackManager) isStreamingURL(outputPath string) bool {
	lower := strings.ToLower(outputPath)
	return strings.HasPrefix(lower, "rtmp://") ||
		strings.HasPrefix(lower, "rtmps://") ||
		strings.HasPrefix(lower, "rtsp://") ||
		strings.HasPrefix(lower, "rtsps://") ||
		strings.HasPrefix(lower, "srt://") ||
		strings.HasPrefix(lower, "udp://") ||
		strings.HasPrefix(lower, "tcp://") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://")
}

// addStreamingFormat adds the appropriate format for streaming URLs
func (fm *FallbackManager) addStreamingFormat(args []string, outputPath string) []string {
	lower := strings.ToLower(outputPath)

	switch {
	case strings.HasPrefix(lower, "rtmp://") || strings.HasPrefix(lower, "rtmps://"):
		args = append(args, "-f", "flv")
	case strings.HasPrefix(lower, "rtsp://") || strings.HasPrefix(lower, "rtsps://"):
		// RTSP publisher/server mode
		args = append(args, "-f", "rtsp")
		// Add RTSP-specific options for publishing
		args = append(args, "-rtsp_transport", "tcp")
		args = append(args, "-muxdelay", "0.1")
		// Optional: Set buffer size for low latency
		args = append(args, "-bufsize", "64k")
		args = append(args, "-maxrate", "2000k")
		// RTSP publisher options
		args = append(args, "-rtsp_flags", "listen")
		args = append(args, "-timeout", "5000000")
		args = append(args, "-stimeout", "5000000")
	case strings.HasPrefix(lower, "srt://"):
		args = append(args, "-f", "mpegts")
	case strings.HasPrefix(lower, "udp://"):
		args = append(args, "-f", "mpegts")
	case strings.HasPrefix(lower, "tcp://"):
		args = append(args, "-f", "mpegts")
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		// For HTTP streaming, check if it's HLS or DASH
		if strings.Contains(lower, ".m3u8") {
			args = append(args, "-f", "hls")
			args = append(args, "-hls_time", "10")
			args = append(args, "-hls_list_size", "0")
		} else if strings.Contains(lower, ".mpd") {
			args = append(args, "-f", "dash")
		} else {
			// Default HTTP streaming format
			args = append(args, "-f", "mpegts")
		}
	default:
		// Fallback to mpegts for unknown streaming protocols
		args = append(args, "-f", "mpegts")
	}

	return args
}

// formatArgsForDisplay joins FFmpeg args into a readable command string
func formatArgsForDisplay(args []string) string {
	var builder strings.Builder
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			builder.WriteString(fmt.Sprintf("\"%s\" ", arg))
		} else {
			builder.WriteString(arg + " ")
		}
	}
	return strings.TrimSpace(builder.String())
}
