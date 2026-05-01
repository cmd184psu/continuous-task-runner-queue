package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cmd184psu/ctrq/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoad_MissingFile(t *testing.T) {
	cfg, err := config.Load("/tmp/nonexistent-ctrq-config-xyz.json")
	require.NoError(t, err)
	require.Equal(t, config.DefaultPort, cfg.Port)
	require.Equal(t, "12345", cfg.Passcode)
	require.True(t, cfg.UIEnabled)
}

func TestLoad_ValidFile(t *testing.T) {
	data := map[string]any{
		"port":       7777,
		"passcode":   "99999",
		"ui_enabled": false,
		"db_path":    "/tmp/test.db",
		"groups": []map[string]any{
			{"name": "batch", "pool_limit": 5},
			{"name": "migrations", "pool_limit": 1, "allowed_types": []string{"migration"}},
		},
	}
	raw, _ := json.Marshal(data)
	f, err := os.CreateTemp("", "ctrq-config-*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, _ = f.Write(raw)
	f.Close()

	cfg, err := config.Load(f.Name())
	require.NoError(t, err)
	require.Equal(t, 7777, cfg.Port)
	require.Equal(t, "99999", cfg.Passcode)
	require.False(t, cfg.UIEnabled)
	require.Equal(t, "/tmp/test.db", cfg.DBPath)
	require.Len(t, cfg.Groups, 2)
	require.Equal(t, "batch", cfg.Groups[0].Name)
	require.Equal(t, 5, cfg.Groups[0].PoolLimit)
	require.Equal(t, "migrations", cfg.Groups[1].Name)
	require.Equal(t, []string{"migration"}, cfg.Groups[1].AllowedTypes)
}

func TestLoad_InvalidJSON(t *testing.T) {
	f, err := os.CreateTemp("", "ctrq-config-*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, _ = f.WriteString("not json {{{")
	f.Close()

	_, err = config.Load(f.Name())
	require.Error(t, err)
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := config.ExpandHome("~/.ctrq.json")
	require.Equal(t, filepath.Join(home, ".ctrq.json"), result)

	result = config.ExpandHome("/absolute/path")
	require.Equal(t, "/absolute/path", result)
}
