// This file is part of the riverdeck main package. See app.go for the package doc.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
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
	Brightness int    `yaml:"brightness"`
	PassiveFPS int    `yaml:"passive_fps"`
	Timeout    int    `yaml:"timeout"`     // Seconds before display sleeps; 0 = never
	Debug      bool   `yaml:"debug"`
	GitBackend string `yaml:"git_backend"` // "auto" | "native" | "go-git"
}

type DeviceConfig struct {
	AutoDetect bool   `yaml:"auto_detect"`
	Path       string `yaml:"path"`
	Model      string `yaml:"model"`
	MultiMode  string `yaml:"multi_mode"` // "shared" | "individual" | "layout"
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
	HTTPTimeout      int    `yaml:"http_timeout"`
	VerifySSL        bool   `yaml:"verify_ssl"`
	WebSocketEnabled bool   `yaml:"websocket_enabled"`
	WebSocketPort    int    `yaml:"websocket_port"` // default 9000
	EditorEnabled    bool   `yaml:"editor_enabled"`
	EditorPort       int    `yaml:"editor_port"` // default 9001
	EditorHost       string `yaml:"editor_host"` // default "127.0.0.1"
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
			GitBackend: "auto",
		},
		Device: DeviceConfig{
			AutoDetect: true,
			Path:       "",
			Model:      "",
			MultiMode:  "shared",
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
			EditorEnabled:    false,
			EditorPort:       9001,
			EditorHost:       "127.0.0.1",
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

// applyEnvOverrides overlays environment variables onto the config struct.
// Env vars use the prefix RIVERDECK_ with __ as the nested-key delimiter:
//
//	RIVERDECK_APPLICATION__BRIGHTNESS=80
//	RIVERDECK_DEVICE__MULTI_MODE=individual
//	RIVERDECK_NETWORK__WEBSOCKET_ENABLED=true
func applyEnvOverrides(cfg *Config) {
	k := koanf.New(".")

	// Seed koanf with current config values so Unmarshal preserves them
	// when no env override exists.
	current, err := yaml.Marshal(cfg)
	if err != nil {
		return
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(current, &raw); err != nil {
		return
	}
	_ = k.Load(confmap.Provider(raw, "."), nil)

	// Overlay environment variables.
	_ = k.Load(env.Provider("RIVERDECK_", "__", func(s string) string {
		return strings.Replace(strings.ToLower(s), "__", ".", -1)
	}), nil)

	_ = k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{Tag: "yaml"})
}

// LoadConfig reads .config.yml from dir, applying defaults and env overrides.
// The directory is created automatically if it does not exist.
func LoadConfig(dir string) (*Config, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	cfg := DefaultConfig()
	data, err := os.ReadFile(platform.ConfigFile(dir))
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

	// Apply environment variable overrides on top of file config.
	applyEnvOverrides(cfg)

	return cfg, nil
}

// LoadDeviceConfig merges a device-level .config.yml on top of a global config.
// Only device-scoped fields (brightness, passive_fps, timeout, nav_style) are
// overridden; all other fields retain their global values.
// Returns a shallow copy so the global config is not mutated.
func LoadDeviceConfig(global *Config, deviceConfigDir string) *Config {
	merged := *global // shallow copy

	path := platform.ConfigFile(deviceConfigDir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &merged
	}
	if err != nil {
		return &merged
	}

	var devCfg struct {
		Application *struct {
			Brightness *int    `yaml:"brightness"`
			PassiveFPS *int    `yaml:"passive_fps"`
			Timeout    *int    `yaml:"timeout"`
		} `yaml:"application"`
		UI *struct {
			NavigationStyle *string            `yaml:"navigation_style"`
			ShowHiddenFiles *bool              `yaml:"show_hidden_files"`
			Labels          map[string]string  `yaml:"labels"`
		} `yaml:"ui"`
	}
	if err := yaml.Unmarshal(data, &devCfg); err != nil {
		return &merged
	}

	if devCfg.Application != nil {
		if devCfg.Application.Brightness != nil {
			merged.Application.Brightness = *devCfg.Application.Brightness
		}
		if devCfg.Application.PassiveFPS != nil {
			merged.Application.PassiveFPS = *devCfg.Application.PassiveFPS
		}
		if devCfg.Application.Timeout != nil {
			merged.Application.Timeout = *devCfg.Application.Timeout
		}
	}
	if devCfg.UI != nil {
		if devCfg.UI.NavigationStyle != nil {
			merged.UI.NavigationStyle = *devCfg.UI.NavigationStyle
		}
		if devCfg.UI.ShowHiddenFiles != nil {
			merged.UI.ShowHiddenFiles = *devCfg.UI.ShowHiddenFiles
		}
		if devCfg.UI.Labels != nil {
			if merged.UI.Labels == nil {
				merged.UI.Labels = devCfg.UI.Labels
			} else {
				for k, v := range devCfg.UI.Labels {
					merged.UI.Labels[k] = v
				}
			}
		}
	}
	return &merged
}

// SaveConfig writes cfg as .config.yml inside dir.
func SaveConfig(cfg *Config, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	f, err := os.Create(platform.ConfigFile(dir))
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
