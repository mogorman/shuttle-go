package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gvalkov/golang-evdev"
)

var configFile = flag.String("config", filepath.Join(os.Getenv("HOME"), ".shuttle-go.json"), "Location to the .shuttle-go.json configuration")
var debugMode = flag.Bool("debug", false, "Show debug messages (like window titles)")
var logFile = flag.String("log-file", "", "Log to a file instead of stdout")
var showVersion = flag.Bool("version", false, "Print the semantic version and exit")

// version is the semantic version, set at build time via
// -ldflags "-X main.version=..." from the committed VERSION file.
var version = "unknown"

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

	if *showVersion {
		fmt.Println(version)
		return
	}

	if *logFile != "" {
		log, err := os.Create(*logFile)
		if err != nil {
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
		os.Exit(10)
	}

	go disableXInputPointer()

	// X-window title change watcher
	watcher := NewWindowWatcher()
	if err := watcher.Setup(); err != nil {
		fmt.Println("Error watching X window:", err)
		os.Exit(3)
	}

	go watcher.Run()

	// IF there's an `osc` driver specified, launch an OSC listener too. It
	// binds a fixed port and must run exactly once for the process lifetime.
	go listenOSCFeedback()

	// Create the virtual uinput device used to emit keyboard and mouse
	// events. It must exist before any binding can fire.
	uinput, err := newUinputDevice()
	if err != nil {
		fmt.Println("Error creating uinput device:", err)
		os.Exit(11)
	}
	defer uinput.Destroy()
	if *debugMode {
		fmt.Println("uinput device ready")
	}
	// Open the device's own /dev/input node so we can read emitted events back
	// (self-test). A non-fatal failure just disables the read-back.
	if err := uinput.openReadNode(); err != nil && *debugMode {
		fmt.Println("uinput read-back unavailable:", err)
	}

	// Wait for the device, then process its events. If the device is unplugged
	// (Process errors), release any held keys, destroy the uinput device, and
	// go back to waiting for the device to be plugged in again.
	for {
		dev := waitForDevice(devicePath)
		if *debugMode {
			fmt.Println("Ready")
		}

		mapper := NewMapper(dev, uinput)
		mapper.watcher = watcher

		for {
			if err := mapper.Process(); err != nil {
				fmt.Println("Lost the Shuttle device (unplugged?):", err)
				mapper.ReleaseAll()
				uinput.Destroy()
				uinput, err = newUinputDevice()
				if err != nil {
					fmt.Println("Error recreating uinput device:", err)
					os.Exit(11)
				}
				break
			}
		}
	}

}
