package config

import (
	"path/filepath"
	"testing"
)

func TestDatabasePathMatchesNodeWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NODE_BACKEND_DIR", dir)
	t.Setenv("GO_SERVER_PORT", "4000")
	t.Setenv("NODE_BACKEND_PORT", "3333")
	for _, configDir := range []string{"relative-config", filepath.Join(t.TempDir(), "config")} {
		t.Setenv("CONFIG_PATH", configDir)
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(configDir, "absdatabase.sqlite")
		if !filepath.IsAbs(configDir) {
			want = filepath.Join(dir, want)
		}
		if cfg.DatabasePath != want {
			t.Fatalf("database path %q, want %q", cfg.DatabasePath, want)
		}
	}
}
