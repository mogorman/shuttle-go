package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// configWatcher watches the config file and, when it changes, fully reloads
// the configuration and re-resolves the active window against the fresh config.
// It runs as its own goroutine (see NewConfigWatcher) so a running shuttle-go
// picks up edits to the config file without a restart.
//
// It prefers inotify for immediate notification and falls back to polling on a
// 5 second interval if inotify is unavailable (non-Linux, or the file is on a
// filesystem inotify cannot watch).
type configWatcher struct {
	path      string
	watcher   *watcher
	lastMod   time.Time
	lastSize  int64
	inotifyFd int // >=0 when inotify is active; -1 when polling
}

// NewConfigWatcher builds a configWatcher for the given config file path.
// watcher is the window watcher used to re-resolve the active app after a
// reload; it may be nil (in which case a reload updates loadedConfiguration but
// leaves currentConfiguration as-is).
func NewConfigWatcher(path string, watcher *watcher) *configWatcher {
	cw := &configWatcher{path: path, watcher: watcher, inotifyFd: -1}
	if st, err := os.Stat(path); err == nil {
		cw.lastMod = st.ModTime()
		cw.lastSize = st.Size()
	}
	return cw
}

// Run blocks, watching the config file and reloading it on change. It uses
// inotify when available and otherwise polls every 5 seconds.
func (cw *configWatcher) Run() {
	if cw.setupInotify() {
		cw.runInotify()
	} else {
		cw.runPolling(5 * time.Second)
	}
}

// setupInotify creates an inotify instance and watches the config file. It
// returns true on success. It watches the file itself (editors that save by
// writing a temp file and renaming it over the original still deliver
// IN_MOVED_TO/IN_CREATE on the watched path).
func (cw *configWatcher) setupInotify() bool {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return false
	}
	mask := uint32(unix.IN_MODIFY | unix.IN_CLOSE_WRITE | unix.IN_CREATE | unix.IN_MOVED_TO)
	if _, err := unix.InotifyAddWatch(fd, cw.path, mask); err != nil {
		unix.Close(fd)
		return false
	}
	cw.inotifyFd = fd
	return true
}

// runInotify blocks reading inotify events. Any relevant event triggers a
// change check (which re-stats and reloads only if mtime/size actually
// changed). If the watch is lost (e.g. the file is replaced and the kernel
// drops the watch) it falls back to polling.
func (cw *configWatcher) runInotify() {
	buf := make([]byte, 4096)
	for {
		n, err := unix.Read(cw.inotifyFd, buf)
		if err != nil {
			// EAGAIN/EWOULDBLOCK is expected with a non-blocking fd and simply
			// means "no event right now"; sleep briefly and retry. Any other
			// error (e.g. EBADF once the watch is gone) drops to polling.
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			cw.teardownInotify()
			cw.runPolling(5 * time.Second)
			return
		}
		if n == 0 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		// At least one event arrived; the file may have changed.
		cw.check()
	}
}

func (cw *configWatcher) teardownInotify() {
	if cw.inotifyFd >= 0 {
		unix.Close(cw.inotifyFd)
		cw.inotifyFd = -1
	}
}

// runPolling blocks, re-checking the file every interval.
func (cw *configWatcher) runPolling(interval time.Duration) {
	for {
		cw.check()
		time.Sleep(interval)
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
	if st.ModTime().Equal(cw.lastMod) && st.Size() == cw.lastSize {
		return
	}

	if *debugMode {
		fmt.Println("Config file changed, reloading:", cw.path)
	}
	if err := LoadConfig(cw.path); err != nil {
		fmt.Println("Error reloading configuration (keeping previous):", err)
		return
	}

	cw.lastMod = st.ModTime()
	cw.lastSize = st.Size()

	if *debugMode {
		fmt.Println("Configuration reloaded")
	}
	if cw.watcher != nil {
		cw.watcher.reapplyWindow()
	}
}
