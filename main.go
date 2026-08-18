package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gvalkov/golang-evdev"
)

var configFile = flag.String("config", filepath.Join(os.Getenv("HOME"), ".shuttle-go.json"), "Location to the .shuttle-go.json configuration")
var debugMode = flag.Bool("debug", false, "Show debug messages (like window titles)")
var logFile = flag.String("log-file", "", "Log to a file instead of stdout")

var startedYdotoold bool

func ensureYdotoold() error {
	if err := exec.Command("pgrep", "-x", "ydotoold").Run(); err == nil {
		return nil
	}
	fmt.Println("Starting ydotoold...")
	startedYdotoold = true
	return exec.Command("ydotoold").Start()
}

func stopYdotoold() {
	if startedYdotoold {
		exec.Command("pkill", "-x", "ydotoold").Run()
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
	fmt.Println("Using device", devicePath)

	if err := LoadConfig(*configFile); err != nil {
		fmt.Println("Error reading configuration:", err)
		stopYdotoold()
		os.Exit(10)
	}

	if err := ensureYdotoold(); err != nil {
		fmt.Println("Error starting ydotoold:", err)
		stopYdotoold()
		os.Exit(11)
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

	// Shuttle device event receiver
	dev, err := evdev.Open(devicePath)
	if err != nil {
		fmt.Println("Couldn't open Shuttle device:", err)
		stopYdotoold()
		os.Exit(2)
	}

	fmt.Println("Ready")
	mapper := NewMapper(dev)
	mapper.watcher = watcher

	// IF there's an `osc` driver specified, launch an OSC listener too:
	go listenOSCFeedback()

	for {
		if err := mapper.Process(); err != nil {
			fmt.Println("Error processing input events (continuing):", err)
			stopYdotoold()
			os.Exit(123)
		}
	}

}
