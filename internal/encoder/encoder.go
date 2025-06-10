package encoder

import (
	"runtime"
	"video_processing/utils"
)

// Encoder handles video encoding configuration
type Encoder struct{}

// New creates a new encoder instance
func New() *Encoder {
	return &Encoder{}
}

// ConfigureForGPU configures encoding settings based on detected GPU
func (e *Encoder) ConfigureForGPU(gpu utils.GPUInfo) (string, string, string) {
	acceleration := e.getAccelerationMethod(gpu)
	codec := e.getCodec(acceleration)
	preset := e.getPreset(acceleration)

	return acceleration, codec, preset
}

func (e *Encoder) getAccelerationMethod(gpu utils.GPUInfo) string {
	switch gpu.Vendor {
	case "nvidia":
		return "cuda"
	case "intel":
		if runtime.GOOS == "windows" {
			return "qsv"
		}
		return "vaapi"
	case "amd":
		if runtime.GOOS == "windows" {
			return "d3d11va"
		}
		return "vaapi"
	case "apple":
		return "videotoolbox"
	default:
		return "none"
	}
}

func (e *Encoder) getCodec(acceleration string) string {
	switch acceleration {
	case "cuda":
		return "h264_nvenc"
	case "qsv":
		return "h264_qsv"
	case "vaapi":
		return "h264_vaapi"
	case "videotoolbox":
		return "h264_videotoolbox"
	case "d3d11va":
		return "h264_amf"
	default:
		return "libx264"
	}
}

func (e *Encoder) getPreset(acceleration string) string {
	switch acceleration {
	case "cuda":
		return "medium"
	case "qsv":
		return "medium"
	case "vaapi":
		return "ultrafast"
	case "videotoolbox":
		return "balanced"
	case "d3d11va":
		return "balanced"
	default:
		return "medium"
	}
}
