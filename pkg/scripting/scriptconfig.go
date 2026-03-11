package scripting

// scriptconfig.go handles loading and saving per-script configuration files.
//
// In folder mode each Lua script may have a sibling .config.json file:
//
//	volume.lua          <- the script
//	volume.config.json  <- its configuration
//
// The JSON format uses the same MetadataField schema as package templates:
//
//	{
//	  "schema": [
//	    {"key": "step", "label": "Volume Step", "type": "number", "default": "5"},
//	    {"key": "target", "label": "Target", "type": "text", "default": ""}
//	  ],
//	  "overrides": {
//	    "step": "10"
//	  }
//	}
//
// This allows the GUI editor to render a property form for any script --
// not just package templates -- and persist user customisations without
// modifying the Lua file itself.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/merith-tk/riverdeck/pkg/scripting/modules"
)

// ScriptConfig is the on-disk format for a per-script configuration file.
type ScriptConfig struct {
	// Schema declares the configurable fields (same shape as package MetadataField).
	Schema []MetadataField `json:"schema"`

	// Overrides stores user-customised values, keyed by schema field key.
	// Only explicitly changed values appear here; absent keys fall back to
	// their schema default.
	Overrides map[string]string `json:"overrides,omitempty"`
}

// ConfigPath returns the expected .config.json path for a given script path.
// e.g. "/config/volume.lua" -> "/config/volume.config.json"
func ConfigPath(scriptPath string) string {
	ext := filepath.Ext(scriptPath)
	return strings.TrimSuffix(scriptPath, ext) + ".config.json"
}

// LoadScriptConfig reads and parses the .config.json file for a script.
// Returns nil (not an error) if the file does not exist.
func LoadScriptConfig(scriptPath string) (*ScriptConfig, error) {
	cfgPath := ConfigPath(scriptPath)
	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg ScriptConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveScriptConfig writes a ScriptConfig to the .config.json path.
func SaveScriptConfig(scriptPath string, cfg *ScriptConfig) error {
	cfgPath := ConfigPath(scriptPath)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

// BuildConfigModule creates a ConfigModule for a script by loading its
// .config.json sibling file.  If no config file exists, an empty module
// is returned (every config.get() returns nil).
//
// For layout-mode scripts where the schema comes from the package template
// and overrides come from the layout button metadata, use
// BuildConfigModuleFromTemplate instead.
func BuildConfigModule(scriptPath string) *modules.ConfigModule {
	cfg, err := LoadScriptConfig(scriptPath)
	if err != nil || cfg == nil {
		return modules.NewConfigModule(nil, nil)
	}
	fields := make([]modules.ConfigField, len(cfg.Schema))
	for i, f := range cfg.Schema {
		fields[i] = modules.ConfigField{
			Key:         f.Key,
			Label:       f.Label,
			Type:        f.Type,
			Default:     f.Default,
			Description: f.Description,
		}
	}
	return modules.NewConfigModule(fields, cfg.Overrides)
}

// BuildConfigModuleFromTemplate creates a ConfigModule from a package
// template's MetadataSchema and a button's metadata overrides map.
// Used in layout mode where config lives in layout.json rather than on disk.
func BuildConfigModuleFromTemplate(schema []MetadataField, overrides map[string]string) *modules.ConfigModule {
	fields := make([]modules.ConfigField, len(schema))
	for i, f := range schema {
		fields[i] = modules.ConfigField{
			Key:         f.Key,
			Label:       f.Label,
			Type:        f.Type,
			Default:     f.Default,
			Description: f.Description,
		}
	}
	return modules.NewConfigModule(fields, overrides)
}
