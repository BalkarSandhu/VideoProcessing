package encoder

import (
	"fmt"
	"path/filepath"
	"strings"
	"video_processing/internal/config"
)

// CommandBuilder builds FFmpeg commands
type CommandBuilder struct{}

// NewCommandBuilder creates a new command builder
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

// BuildFFmpegCommand builds the complete FFmpeg command arguments
func (cb *CommandBuilder) BuildFFmpegCommand(config *config.ProcessingConfig) []string {
	var args []string

	// Hardware acceleration setup
	args = cb.addHardwareAcceleration(args, config.Acceleration)

	// Input
	args = append(args, "-i", config.InputPath)

	// Video encoding
	args = cb.addVideoEncoding(args, config)

	// Audio (copy without re-encoding)
	args = append(args, "-c:a", "copy")

	// Output format based on URL/path
	args = cb.addOutputFormat(args, config.OutputPath)

	// Output options
	args = append(args, "-movflags", "+faststart") // Web optimization
	args = append(args, "-bf", "0")
	args = append(args, "-fflags", "nobuffer")
	args = append(args, "-flags", "low_delay")
	args = append(args, "-fflags", "+discardcorrupt")
	args = append(args, "-analyzeduration", "0")
	args = append(args, "-probesize", "32")
	args = append(args, "-tune", "zerolatency")

	args = append(args, "-y") // Overwrite output
	args = append(args, config.OutputPath)

	return args
}

func (cb *CommandBuilder) addHardwareAcceleration(args []string, acceleration string) []string {
	switch acceleration {
	case "cuda":
		args = append(args, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
	case "qsv":
		args = append(args, "-hwaccel", "qsv")
	case "vaapi":
		args = append(args, "-init_hw_device", "vaapi=va:/dev/dri/renderD128")
		args = append(args, "-filter_hw_device", "va")
		args = append(args, "-hwaccel_output_format", "vaapi")
		// args = append(args, "-hwaccel", "vaapi")
		// args = append(args, "-hwaccel_device", "/dev/dri/renderD128")
		// args = append(args, "-hwaccel_output_format", "vaapi")
	case "videotoolbox":
		args = append(args, "-hwaccel", "videotoolbox")
	case "d3d11va":
		args = append(args, "-hwaccel", "d3d11va")
	}
	return args
}

func (cb *CommandBuilder) addVideoEncoding(args []string, config *config.ProcessingConfig) []string {
	switch config.Codec {
	case "h264_nvenc":
		args = append(args, "-c:v", config.Codec)
		args = append(args, "-preset", config.Preset)
		args = append(args, "-rc", "vbr", "-cq", fmt.Sprintf("%d", config.Quality))
		args = append(args, "-b:v", "0") // Use CQ mode
	case "h264_qsv":
		args = append(args, "-c:v", config.Codec)
		args = append(args, "-preset", config.Preset)
		args = append(args, "-global_quality", fmt.Sprintf("%d", config.Quality))
	case "h264_vaapi":
		args = append(args, "-vf", "format=nv12,hwupload")
		args = append(args, "-c:v", config.Codec)
		args = append(args, "-qp", fmt.Sprintf("%d", config.Quality))
	case "h264_videotoolbox":
		args = append(args, "-c:v", config.Codec)
		args = append(args, "-q:v", fmt.Sprintf("%d", config.Quality))
	case "h264_amf":
		args = append(args, "-c:v", config.Codec)
		args = append(args, "-quality", config.Preset)
		args = append(args, "-rc", "cqp")
		args = append(args, "-qp_i", fmt.Sprintf("%d", config.Quality))
		args = append(args, "-qp_p", fmt.Sprintf("%d", config.Quality))
	default: // libx264
		args = append(args, "-c:v", "libx264")
		args = append(args, "-preset", config.Preset)
		args = append(args, "-crf", fmt.Sprintf("%d", config.Quality))
	}
	return args
}

// addOutputFormat adds the appropriate output format based on the output path/URL
func (cb *CommandBuilder) addOutputFormat(args []string, outputPath string) []string {
	// Check if it's a streaming URL
	if cb.isStreamingURL(outputPath) {
		return cb.addStreamingFormat(args, outputPath)
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

// isStreamingURL checks if the output path is a streaming URL
func (cb *CommandBuilder) isStreamingURL(outputPath string) bool {
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
func (cb *CommandBuilder) addStreamingFormat(args []string, outputPath string) []string {
	lower := strings.ToLower(outputPath)

	switch {
	case strings.HasPrefix(lower, "rtmp://") || strings.HasPrefix(lower, "rtmps://"):
		args = append(args, "-f", "flv")
	case strings.HasPrefix(lower, "rtsp://") || strings.HasPrefix(lower, "rtsps://"):
		args = append(args, "-f", "rtsp")
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
