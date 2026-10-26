//go:build windows

package focus

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrInvalidConfig = errors.New("invalid perusal config")

// AgentDir and OwnerDir are one directory, as each account reaches it.
type Config struct {
	AgentAccount string `json:"agentAccount"`
	FocusFile    string `json:"focusFile"`
	AgentDir     string `json:"agentDir"`
	OwnerDir     string `json:"ownerDir"`
}

func LoadConfig(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("%w: parse %s: %w", ErrInvalidConfig, path, err)
	}
	if config.AgentAccount == "" {
		return Config{}, fmt.Errorf("%w: %s sets no agentAccount", ErrInvalidConfig, path)
	}
	paths := []struct {
		key   string
		value string
	}{
		{key: "focusFile", value: config.FocusFile},
		{key: "agentDir", value: config.AgentDir},
		{key: "ownerDir", value: config.OwnerDir},
	}
	for _, configured := range paths {
		if !filepath.IsAbs(configured.value) {
			return Config{}, fmt.Errorf("%w: %s sets %s to %q, want an absolute path", ErrInvalidConfig, path,
				configured.key, configured.value)
		}
	}
	return config, nil
}
