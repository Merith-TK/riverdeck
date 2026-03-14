package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/merith-tk/riverdeck/pkg/platform"
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
	HTTPTimeout      int  `yaml:"http_timeout"`
	VerifySSL        bool `yaml:"verify_ssl"`
	WebSocketEnabled bool `yaml:"websocket_enabled"`
	WebSocketPort    int  `yaml:"websocket_port"` // default 9000
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
			HTTPTimeout:      10,
			VerifySSL:        true,
			WebSocketEnabled: false,
			WebSocketPort:    9000,
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

// ConfigDir returns the configuration directory to use.
// Delegates to the shared platform.ConfigDir() implementation.
//
// If override is non-empty it is returned as-is; otherwise the platform default:
//
//	Windows : %APPDATA%\.riverdeck
//	Other   : $HOME/.config/riverdeck
func ConfigDir(override string) string {
	return platform.ConfigDir(override)
}

// LoadConfig reads config.yml from dir, creating it with defaults when absent.
// The directory is created automatically if it does not exist.
func LoadConfig(dir string) (*Config, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	cfg := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if os.IsNotExist(err) {
		// First run -- write defaults so the user has a file to edit.
		if werr := SaveConfig(cfg, dir); werr != nil {
			return cfg, fmt.Errorf("failed to write default config: %w", werr)
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("failed to read config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config: %w", err)
	}
	return cfg, nil
}

// SaveConfig writes cfg as config.yml inside dir.
func SaveConfig(cfg *Config, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	f, err := os.Create(filepath.Join(dir, "config.yml"))
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return enc.Close()
}
