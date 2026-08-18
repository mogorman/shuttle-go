package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gvalkov/golang-evdev"
)

var configFile = flag.String("config", filepath.Join(os.Getenv("HOME"), ".shuttle-go.json"), "Location to the .shuttle-go.json configuration")
var debugMode = flag.Bool("debug", false, "Show debug messages (like window titles)")
var logFile = flag.String("log-file", "", "Log to a file instead of stdout")

var startedYdotoold bool

// ydotooldRunning reports whether a ydotoold process is currently alive.
func ydotooldRunning() bool {
	return exec.Command("pgrep", "-x", "ydotoold").Run() == nil
}

// ensureYdotoold starts ydotoold if it is not running. It is a no-op when a
// live ydotoold is already present.
func ensureYdotoold() error {
	if ydotooldRunning() {
		return nil
	}
	if *debugMode {
		fmt.Println("Starting ydotoold...")
	}
	startedYdotoold = true
	if err := exec.Command("ydotoold").Start(); err != nil {
		return err
	}
	// Wait until it is actually up so callers can rely on it immediately.
	for i := 0; i < 50 && !ydotooldRunning(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// stopYdotoold kills ydotoold (only if we started it) and waits for it to
// actually exit, so a subsequent ensureYdotoold does not mistake a dying
// process for a live one.
func stopYdotoold() {
	if !startedYdotoold {
		return
	}
	exec.Command("pkill", "-x", "ydotoold").Run()
	for i := 0; i < 50 && ydotooldRunning(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

// waitForDevice blocks until the Shuttle device can be opened, retrying every
// 5 seconds. It returns a freshly opened device (so a re-plugged device gets a
// clean file handle).
func waitForDevice(devicePath string) *evdev.InputDevice {
	for {
		dev, err := evdev.Open(devicePath)
		if err == nil {
			return dev
		}
		fmt.Println("Shuttle device not available:", err)
		fmt.Println("Waiting for the device to be plugged in (retrying every 5s)...")
		time.Sleep(5 * time.Second)
	}
}

func main() {
	flag.Parse()

	if *logFile != "" {
		log, err := os.Create(*logFile)
		if err != nil {
			stopYdotoold()
			os.Exit(101)
		}
		defer log.Close()
		os.Stderr = log
		os.Stdout = log
	}

	devicePath := "/dev/input/by-id/usb-Contour_Design_ShuttlePRO_v2-event-mouse"
	if len(flag.Args()) == 1 {
		devicePath = flag.Arg(0)
	}
	if *debugMode {
		fmt.Println("Using device", devicePath)
	}

	if err := LoadConfig(*configFile); err != nil {
		fmt.Println("Error reading configuration:", err)
		stopYdotoold()
		os.Exit(10)
	}

	go disableXInputPointer()

	// X-window title change watcher
	watcher := NewWindowWatcher()
	if err := watcher.Setup(); err != nil {
		fmt.Println("Error watching X window:", err)
		stopYdotoold()
		os.Exit(3)
	}

	go watcher.Run()

	// IF there's an `osc` driver specified, launch an OSC listener too. It
	// binds a fixed port and must run exactly once for the process lifetime.
	go listenOSCFeedback()

	// Wait for the device, then process its events. If the device is unplugged
	// (Process errors), release any held keys, stop ydotoold if we started it,
	// and go back to waiting for the device to be plugged in again.
	for {
		// (Re)acquire ydotoold at the top of every iteration: it's a no-op if
		// already running, and restarts it after we killed it on an unplug.
		if err := ensureYdotoold(); err != nil {
			fmt.Println("Error starting ydotoold:", err)
			stopYdotoold()
			os.Exit(11)
		}

		dev := waitForDevice(devicePath)
		if *debugMode {
			fmt.Println("Ready")
		}

		mapper := NewMapper(dev)
		mapper.watcher = watcher

		for {
			if err := mapper.Process(); err != nil {
				fmt.Println("Lost the Shuttle device (unplugged?):", err)
				mapper.ReleaseAll()
				stopYdotoold()
				break
			}
		}
	}

}
