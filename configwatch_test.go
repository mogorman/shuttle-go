package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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

// TestConfigWatcherInotifyDelivers verifies the inotify watch is set up and
// that a real edit to the file produces a readable event on the watch fd. It
// skips on non-Linux platforms where inotify is unavailable.
func TestConfigWatcherInotifyDelivers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inotify is Linux-only")
	}

	path := writeConfigFile(t, `{"apps":[{"name":"A","bindings":{}}]}`)
	cw := NewConfigWatcher(path, nil)
	if !cw.setupInotify() {
		t.Fatal("expected inotify setup to succeed on linux")
	}
	defer cw.teardownInotify()

	// Rewrite the file; this should enqueue an inotify event on the watch fd.
	if err := os.WriteFile(path, []byte(`{"apps":[{"name":"C","bindings":{}}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4096)
	n, err := unix.Read(cw.inotifyFd, buf)
	if err != nil {
		t.Fatalf("reading inotify event: %v", err)
	}
	if n < unix.SizeofInotifyEvent {
		t.Fatalf("expected at least one inotify event, read %d bytes", n)
	}
}
