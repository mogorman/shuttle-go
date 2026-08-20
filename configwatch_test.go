package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeConfigFile writes content to a fresh temp file and returns its path.
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shuttle-go.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigWatcherNoChangeIsNoOp(t *testing.T) {
	path := writeConfigFile(t, `{"apps":[{"name":"A","bindings":{}}]}`)
	if err := LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	before := loadedConfiguration

	cw := NewConfigWatcher(path, nil)
	cw.check()

	if loadedConfiguration != before {
		t.Fatal("expected no reload when the config file is unchanged")
	}
}

func TestConfigWatcherReloadsOnChange(t *testing.T) {
	path := writeConfigFile(t, `{"apps":[{"name":"A","bindings":{}}]}`)
	if err := LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	before := loadedConfiguration

	// Construct the watcher against the as-loaded file (as the real flow does),
	// then edit the file and force a newer mtime so the change is detected even
	// on coarse clocks.
	cw := NewConfigWatcher(path, nil)

	future := time.Now().Add(time.Minute)
	if err := os.WriteFile(path, []byte(`{"apps":[{"name":"B","bindings":{}}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Now(), future); err != nil {
		t.Fatal(err)
	}

	cw.check()

	if loadedConfiguration == before {
		t.Fatal("expected a reload after the config file changed")
	}
	if len(loadedConfiguration.Apps) != 1 || loadedConfiguration.Apps[0].Name != "B" {
		t.Fatalf("expected reloaded app B, got %+v", loadedConfiguration.Apps)
	}
}

func TestConfigWatcherKeepsPreviousOnInvalidEdit(t *testing.T) {
	path := writeConfigFile(t, `{"apps":[{"name":"A","bindings":{}}]}`)
	if err := LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	before := loadedConfiguration

	cw := NewConfigWatcher(path, nil)

	future := time.Now().Add(time.Minute)
	if err := os.WriteFile(path, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Now(), future); err != nil {
		t.Fatal(err)
	}

	cw.check()

	if loadedConfiguration != before {
		t.Fatal("expected the previous config to be kept after an invalid edit")
	}
}
