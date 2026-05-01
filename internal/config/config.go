package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmd184psu/ctrq/internal/models"
)

const DefaultConfigPath = "~/.ctrq.json"
const DefaultPort = 9898

func DefaultConfig() *models.Config {
	return &models.Config{
		Port:      DefaultPort,
		Passcode:  "12345",
		UIEnabled: true,
		DBPath:    "~/.ctrq.db",
		Groups:    []models.GroupConfig{},
	}
}

func Load(path string) (*models.Config, error) {
	path = ExpandHome(path)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.DBPath = ExpandHome(cfg.DBPath)
	return cfg, nil
}

func ExpandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
