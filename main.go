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
var testEmit = flag.String("test", "", "Emit a known key sequence and exit (e.g. 'test' emits a, b, c, d, e) for verifying uinput delivery")
var holdDevice = flag.Bool("hold", false, "Create the uinput device, report the grab state, and keep it alive until interrupted")

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

	// --test: create the uinput device and emit a known key sequence, then
	// exit. This lets you verify event delivery with an independent reader
	// (e.g. `evtest`) without touching the Shuttle. The default sequence is
	// a, b, c, d, e (key codes 30, 48, 46, 32, 18).
	if *testEmit != "" {
		uinput, err := newUinputDevice()
		if err != nil {
			fmt.Println("Error creating uinput device:", err)
			os.Exit(11)
		}
		defer uinput.Destroy()
		fmt.Println("uinput device ready (test mode)")
		fmt.Println("grab check:", uinput.checkGrab())
		fmt.Println("Now run `evtest` in another terminal, select the shuttle-go-virtual device, and press Enter to start reading.")
		fmt.Println("Waiting 10s before emitting so you have time to select the device and start evtest...")
		time.Sleep(10 * time.Second)
		codes := []int{30, 48, 46, 32, 18} // a, b, c, d, e
		for i, c := range codes {
			if err := uinput.KeyTap([]int{c}); err != nil {
				fmt.Println("Error emitting test key:", err)
				os.Exit(12)
			}
			fmt.Printf("emitted test key %d (%c)\n", c, "abcde"[i])
			time.Sleep(1 * time.Second)
		}
		fmt.Println("Keys emitted. Holding the device for 10s more so evtest can keep reading...")
		time.Sleep(10 * time.Second)
		fmt.Println("Done. If evtest showed EV_KEY events for codes 30,48,46,32,18, delivery is working.")
		return
	}

	// --hold: create the uinput device, report the grab state, then keep it
	// alive (emitting nothing) until interrupted. Use this to run a long-lived
	// device while you inspect it with evtest or check who is grabbing it.
	if *holdDevice {
		uinput, err := newUinputDevice()
		if err != nil {
			fmt.Println("Error creating uinput device:", err)
			os.Exit(11)
		}
		defer uinput.Destroy()
		fmt.Println("uinput device ready (hold mode); it will stay alive until you press Ctrl-C")
		fmt.Println("grab check:", uinput.checkGrab())
		fmt.Println("Inspect it now (e.g. `sudo evtest`, select the shuttle-go-virtual node).")
		select {}
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
