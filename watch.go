package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgb/xtest"
)

type watcher struct {
	conn                 *xgb.Conn
	root                 xproto.Window
	activeAtom, nameAtom xproto.Atom
	prevWindowName       string
	lastWindowID         xproto.Window
}

func NewWindowWatcher() *watcher {
	return &watcher{}
}

func getWaylandWindowTitle() string {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return ""
	}

	// GNOME Shell "Windows" extension: list all windows as JSON
	cmd := exec.Command("dbus-send", "--session", "--print-reply=literal",
		"--dest=org.gnome.Shell",
		"/org/gnome/Shell/Extensions/Windows",
		"org.gnome.Shell.Extensions.Windows.List")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return ""
	}

	type window struct {
		Title string `json:"title"`
		Focus bool   `json:"focus"`
	}
	var windows []window
	if json.Unmarshal(out, &windows) == nil {
		for _, w := range windows {
			if w.Focus {
				return w.Title
			}
		}
	}

	return ""
}

func (w *watcher) Setup() error {
	if isWayland() {
		// No X server on Wayland; window-title matching is driven by
		// getWaylandWindowTitle() in watch().
		return nil
	}

	X, err := xgb.NewConn()
	if err != nil {
		return err
	}

	// Get the window id of the root window.
	setup := xproto.Setup(X)

	if err := xtest.Init(X); err != nil {
		return err
	}

	w.conn = X
	w.root = setup.DefaultScreen(X).Root

	// Get the atom id (i.e., intern an atom) of "_NET_ACTIVE_WINDOW".
	aname := "_NET_ACTIVE_WINDOW"
	activeAtom, err := xproto.InternAtom(X, true, uint16(len(aname)),
		aname).Reply()
	if err != nil {
		return fmt.Errorf("Couldn't get _NET_ACTIVE_WINDOW atom: %s", err)
	}

	// Get the atom id (i.e., intern an atom) of "_NET_WM_NAME".
	aname = "_NET_WM_NAME"
	nameAtom, err := xproto.InternAtom(X, true, uint16(len(aname)),
		aname).Reply()
	if err != nil {
		return fmt.Errorf("Couldn't get _NET_WM_NAME atom: %s", err)
	}

	w.activeAtom = activeAtom.Atom
	w.nameAtom = nameAtom.Atom

	return nil
}

func (w *watcher) Run() {
	for {
		w.watch()
		time.Sleep(2 * time.Second)
	}
}

func (w *watcher) watch() {
	var windowName string

	if isWayland() {
		windowName = getWaylandWindowTitle()
		if windowName == "" {
			return
		}
	} else {
		// From github.com/BurntSushi/xgb's examples.
		reply, err := xproto.GetProperty(w.conn, false, w.root, w.activeAtom,
			xproto.GetPropertyTypeAny, 0, (1<<32)-1).Reply()
		if err != nil {
			fmt.Println("watch windows, failed to get window properties:", err)
			return
		}
		windowID := xproto.Window(xgb.Get32(reply.Value))

		reply, err = xproto.GetProperty(w.conn, false, windowID, w.nameAtom,
			xproto.GetPropertyTypeAny, 0, (1<<32)-1).Reply()
		if err != nil {
			fmt.Println("watch windows, re-failed to get window properties:", err)
			return
		}

		w.lastWindowID = windowID
		windowName = string(reply.Value)
	}

	if *debugMode {
		fmt.Println("Active window title:", windowName)
	}
	if w.prevWindowName != windowName {
		w.prevWindowName = windowName

		w.loadWindowConfiguration(windowName)
	}
}

func (w *watcher) loadWindowConfiguration(windowName string) {
	if loadedConfiguration == nil {
		fmt.Println("Window name switched, but no configuration:", windowName)
		return
	}

	for _, conf := range loadedConfiguration.Apps {
		for _, re := range conf.windowTitleRegexps {
			if *debugMode {
				fmt.Println("Testing title:", windowName)
			}
			if re.MatchString(windowName) {
				fmt.Printf("Switching configuration for app %q\n", conf.Name)
				currentConfiguration = conf
				return
			}
		}
	}

	if !*debugMode {
		currentConfiguration = nil
	} else {
		fmt.Println("Keeping previous config even if window changed")
	}
}
