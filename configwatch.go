package main

import (
	"fmt"
	"os"
	"time"
)

// configWatcher polls the config file's modification time and, when it
// changes, fully reloads the configuration and re-resolves the active window
// against the fresh config. It runs as its own goroutine (see
// NewConfigWatcher) so a running shuttle-go picks up edits to the config file
// without a restart.
type configWatcher struct {
	path        string
	watcher     *watcher
	lastModTime time.Time
	lastSize    int64
}

// NewConfigWatcher builds a configWatcher for the given config file path.
// watcher is the window watcher used to re-resolve the active app after a
// reload; it may be nil (in which case a reload updates loadedConfiguration but
// leaves currentConfiguration as-is).
func NewConfigWatcher(path string, watcher *watcher) *configWatcher {
	cw := &configWatcher{path: path, watcher: watcher}
	if st, err := os.Stat(path); err == nil {
		cw.lastModTime = st.ModTime()
		cw.lastSize = st.Size()
	}
	return cw
}

// Run blocks, polling the config file every second and reloading it on change.
func (cw *configWatcher) Run() {
	for {
		cw.check()
		time.Sleep(1 * time.Second)
	}
}

// check reloads the config if the file's mtime or size changed since the last
// check. A failed reload (e.g. a transiently invalid edit) is reported but not
// fatal: the previous config stays in effect and the next tick retries.
func (cw *configWatcher) check() {
	st, err := os.Stat(cw.path)
	if err != nil {
		return
	}
	if st.ModTime().Equal(cw.lastModTime) && st.Size() == cw.lastSize {
		return
	}

	if *debugMode {
		fmt.Println("Config file changed, reloading:", cw.path)
	}
	if err := LoadConfig(cw.path); err != nil {
		fmt.Println("Error reloading configuration (keeping previous):", err)
		return
	}

	cw.lastModTime = st.ModTime()
	cw.lastSize = st.Size()

	if *debugMode {
		fmt.Println("Configuration reloaded")
	}
	if cw.watcher != nil {
		cw.watcher.reapplyWindow()
	}
}
