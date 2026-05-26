package wsclient

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func LoadOrCreateDeviceID(configDir, fileName string) string {
	path := filepath.Join(configDir, fileName)
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	id := uuid.New().String()
	_ = os.WriteFile(path, []byte(id+"\n"), 0644)
	return id
}
