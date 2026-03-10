package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds application configuration.
type Config struct {
	Application ApplicationConfig `yaml:"application"`
	Device      DeviceConfig      `yaml:"device"`
	Scripting   ScriptingConfig   `yaml:"scripting"`
	UI          UIConfig          `yaml:"ui"`
	Performance PerformanceConfig `yaml:"performance"`
	Network     NetworkConfig     `yaml:"network"`
	Logging     LoggingConfig     `yaml:"logging"`
	Security    SecurityConfig    `yaml:"security"`
}

type ApplicationConfig struct {
	Brightness int  `yaml:"brightness"`
	PassiveFPS int  `yaml:"passive_fps"`
	Timeout    int  `yaml:"timeout"` // Seconds before display sleeps; 0 = never
	Debug      bool `yaml:"debug"`
}

type DeviceConfig struct {
	AutoDetect bool   `yaml:"auto_detect"`
	Path       string `yaml:"path"`
	Model      string `yaml:"model"`
}

type ScriptingConfig struct {
	EnableBackground     bool `yaml:"enable_background"`
	ExecutionTimeout     int  `yaml:"execution_timeout"`
	MaxConcurrentScripts int  `yaml:"max_concurrent_scripts"`
}

type UIConfig struct {
	NavigationStyle string            `yaml:"navigation_style"`
	ShowHiddenFiles bool              `yaml:"show_hidden_files"`
	Labels          map[string]string `yaml:"labels"`
}

type PerformanceConfig struct {
	ImageCacheSize int  `yaml:"image_cache_size"`
	CompressImages bool `yaml:"compress_images"`
	JPEGQuality    int  `yaml:"jpeg_quality"`
}

type NetworkConfig struct {
	HTTPTimeout int  `yaml:"http_timeout"`
	VerifySSL   bool `yaml:"verify_ssl"`
}

type LoggingConfig struct {
	Level       string `yaml:"level"`
	File        string `yaml:"file"`
	MaxFileSize int    `yaml:"max_file_size"`
	MaxFiles    int    `yaml:"max_files"`
}

type SecurityConfig struct {
	RestrictFileAccess bool     `yaml:"restrict_file_access"`
	AllowedCommands    []string `yaml:"allowed_commands"`
	BlockNetwork       bool     `yaml:"block_network"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Application: ApplicationConfig{
			Brightness: 75,
			PassiveFPS: 30,
			Timeout:    0,
			Debug:      false,
		},
		Device: DeviceConfig{
			AutoDetect: true,
			Path:       "",
			Model:      "",
		},
		Scripting: ScriptingConfig{
			EnableBackground:     true,
			ExecutionTimeout:     30,
			MaxConcurrentScripts: 10,
		},
		UI: UIConfig{
			NavigationStyle: "folder",
			ShowHiddenFiles: false,
			Labels: map[string]string{
				"back": "<-",
				"home": "HOME",
			},
		},
		Performance: PerformanceConfig{
			ImageCacheSize: 50,
			CompressImages: true,
			JPEGQuality:    90,
		},
		Network: NetworkConfig{
			HTTPTimeout: 10,
			VerifySSL:   true,
		},
		Logging: LoggingConfig{
			Level:       "info",
			File:        "",
			MaxFileSize: 10,
			MaxFiles:    5,
		},
		Security: SecurityConfig{
			RestrictFileAccess: true,
			AllowedCommands:    []string{},
			BlockNetwork:       false,
		},
	}
}

// LoadConfig loads configuration from the config file.
func LoadConfig(configDir string) (*Config, error) {
	configPath := filepath.Join(configDir, "config.yml")

	// Start with defaults
	config := DefaultConfig()

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Save defaults to create the file
		if err := SaveConfig(config, configPath); err != nil {
			return config, fmt.Errorf("failed to create default config: %w", err)
		}
		return config, nil
	}

	// Read and parse config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return config, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// SaveConfig saves configuration to the config file.
func SaveConfig(config *Config, configPath string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// getConfigPath determines the configuration path.
// Priority order:
// 1. --configdir flag (if provided)
// 2. ./.riverdeck directory in current working directory
// 3. ~/.riverdeck directory in user home
func getConfigPath(configDir string) string {
	// 1. Use --configdir if provided
	if configDir != "" {
		return configDir
	}

	// 2. Check for .riverdeck directory in current path
	if info, err := os.Stat(".riverdeck"); err == nil && info.IsDir() {
		return ".riverdeck"
	}

	// 3. Fall back to ~/.riverdeck
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Consider returning error or using a temp directory
		return ".riverdeck"
	}

	return filepath.Join(homeDir, ".riverdeck")
}

// ensureConfigDir creates the configuration directory if it doesn't exist.
func ensureConfigDir(configPath string) (string, error) {
	if err := os.MkdirAll(configPath, 0755); err != nil {
		return "", err
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}

	return absConfigPath, nil
}
