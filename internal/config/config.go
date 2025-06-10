package config

// ProcessingConfig holds all configuration for video processing
type ProcessingConfig struct {
	Acceleration string
	Codec        string
	Quality      int
	Preset       string
	InputPath    string
	OutputPath   string
}

// NewDefault creates a new config with default values
func NewDefault() *ProcessingConfig {
	return &ProcessingConfig{
		Quality:    23, // Default CRF/QP value
		OutputPath: "output.mp4",
	}
}

// SetSoftwareEncoding configures the config for software encoding
func (c *ProcessingConfig) SetSoftwareEncoding() {
	c.Acceleration = "none"
	c.Codec = "libx264"
	c.Preset = "medium"
}

// SetHardwareEncoding configures the config for hardware encoding
func (c *ProcessingConfig) SetHardwareEncoding(acceleration, codec, preset string) {
	c.Acceleration = acceleration
	c.Codec = codec
	c.Preset = preset
}
