// Command uinput-selftest is a standalone, self-contained test of the Linux
// uinput event-emission path. It:
//
//  1. opens /dev/uinput
//  2. registers a minimal keyboard (EV_KEY, a handful of keys)
//  3. creates the virtual device
//  4. taps a known key (a, code 30)
//  5. reads the events back through the device's own /dev/input node
//
// If step 5 prints the press/release/SYN events, then uinput emission is
// proven correct end-to-end on this machine, independent of shuttle-go's
// mapper, config, window-watching, or the physical Shuttle. Run as root:
//
//	sudo go run ./cmd/uinput-selftest
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// input event types (linux/input.h)
const (
	evSyn  = 0x00
	evKey  = 0x01
	evRel  = 0x02
	evAbs  = 0x03
)

// uinput ioctls (linux/uinput.h)
const (
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
	uiSetEvBit   = 0x40045564 // _IOW('U', 100, int)
	uiSetKeyBit  = 0x40045565 // _IOW('U', 101, int)
	uiSetRelBit  = 0x40045566 // _IOW('U', 102, int)
)

// evIOCGRAB is _IOW('E', 0x90, int) (linux/input.h).
const evIOCGRAB = 0x40044590

const (
	uinputMaxNameSize = 80
	absCnt            = 0x40 // ABS_CNT
	keyMax            = 0x2ff
)

// uinputUserDev mirrors struct uinput_user_dev (linux/uinput.h).
type uinputUserDev struct {
	name         [uinputMaxNameSize]byte
	bustype      uint16
	vendor       uint16
	product      uint16
	version      uint16
	ffEffectsMax uint32
	absmax       [absCnt]int32
	absmin       [absCnt]int32
	absfuzz      [absCnt]int32
	absflat      [absCnt]int32
}

func main() {
	fmt.Println("== uinput-selftest: creating a virtual keyboard ==")

	f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0)
	if err != nil {
		fatal("opening /dev/uinput: %v (are you root?)", err)
	}
	defer f.Close()
	fd := int(f.Fd())

	// Register a minimal keyboard: EV_KEY for a few keys. We register the full
	// range so any key can be emitted; this is what the real driver does too.
	must(ioctl(fd, uiSetEvBit, uintptr(evKey)), "UI_SET_EVBIT(EV_KEY)")
	for code := 0; code <= keyMax; code++ {
		must(ioctl(fd, uiSetKeyBit, uintptr(code)), "UI_SET_KEYBIT(%d)", code)
	}

	// Write the uinput_user_dev blob and create the device.
	var dev uinputUserDev
	copy(dev.name[:], "uinput-selftest-kbd")
	dev.bustype = 0x03 // BUS_USB
	dev.vendor = 0x0001
	dev.product = 0x0002
	dev.version = 0x0100
	if _, err := f.Write((*[unsafe.Sizeof(dev)]byte)(unsafe.Pointer(&dev))[:]); err != nil {
		fatal("writing uinput_user_dev: %v", err)
	}
	must(ioctl(fd, uiDevCreate, 0), "UI_DEV_CREATE")
	fmt.Println("   device created")

	// Locate the device's /dev/input/eventN node by matching input_id.
	node := findNode(dev.bustype, dev.vendor, dev.product, dev.version)
	fmt.Println("   device node:", orDash(node))

	// Tap a key: press (value 1), SYN, release (value 0), SYN.
	const key = 30 // 'a'
	fmt.Printf("== emitting key %d ('a'): press, SYN, release, SYN ==\n", key)
	must(writeEvent(fd, evKey, uint16(key), 1), "write press")
	must(writeEvent(fd, evSyn, 0, 0), "write syn 1")
	must(writeEvent(fd, evKey, uint16(key), 0), "write release")
	must(writeEvent(fd, evSyn, 0, 0), "write syn 2")
	fmt.Println("   all 4 writes returned success")

	// Now read the events back through the device's own node. We open it and
	// take an exclusive grab so we are guaranteed to receive what we emit.
	if node == "" {
		fmt.Println("== could not find the device node; cannot self-read ==")
		fmt.Println("   (writes succeeded; if evtest also sees nothing, the kernel is dropping events)")
		return
	}
	rf, err := os.OpenFile(node, os.O_RDONLY, 0)
	if err != nil {
		fatal("opening %s for read-back: %v", node, err)
	}
	defer rf.Close()
	rfd := int(rf.Fd())

	var grabArg int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(rfd), uintptr(evIOCGRAB),
		uintptr(unsafe.Pointer(&grabArg))); errno != 0 {
		fmt.Printf("   (grab on reader fd failed: %s; will still try to read)\n", errno)
	} else {
		defer syscall.Syscall(syscall.SYS_IOCTL, uintptr(rfd), uintptr(evIOCGRAB), 0)
	}

	fmt.Println("== reading back through our own fd (expect 4 events) ==")
	got := 0
	for i := 0; i < 8; i++ {
		t, c, v, n, err := readEvent(rfd)
		if err != nil {
			fmt.Printf("   read stopped: %v (got %d events so far)\n", err, got)
			break
		}
		if n == 0 {
			break
		}
		got++
		fmt.Printf("   type=%d code=%d value=%d\n", t, c, v)
	}

	fmt.Println()
	if got >= 3 {
		fmt.Println("RESULT: PASS — uinput emission works end-to-end on this machine.")
		fmt.Println("        The kernel receives and re-emits our events. Any failure in")
		fmt.Println("        shuttle-go is therefore about focus/permissions, not emission.")
	} else {
		fmt.Println("RESULT: FAIL — the kernel dropped our events even to a grabbing reader.")
		fmt.Println("        This points to a uinput/evdev or seat-grab problem on this box.")
	}
}

// --- helpers ---

func fatal(format string, args ...interface{}) {
	fmt.Printf("FATAL: "+format+"\n", args...)
	os.Exit(1)
}

func orDash(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func must(err error, format string, args ...interface{}) {
	if err != nil {
		fatal(format, args...)
	}
}

func ioctl(fd int, cmd, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), cmd, arg)
	if errno != 0 {
		return errno
	}
	return nil
}

// writeEvent writes one 24-byte input_event.
func writeEvent(fd int, evType, code uint16, value int32) error {
	var ev [24]byte
	binary.LittleEndian.PutUint64(ev[0:8], 0) // t_sec
	binary.LittleEndian.PutUint32(ev[8:12], 0) // t_usec
	binary.LittleEndian.PutUint16(ev[12:14], evType)
	binary.LittleEndian.PutUint16(ev[14:16], code)
	binary.LittleEndian.PutUint32(ev[16:20], uint32(value))
	n, _, errno := syscall.Syscall6(syscall.SYS_WRITE, uintptr(fd),
		uintptr(unsafe.Pointer(&ev[0])), uintptr(len(ev)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("write: %s (wrote %d)", errno, n)
	}
	if n != uintptr(len(ev)) {
		return fmt.Errorf("short write: %d of %d", n, len(ev))
	}
	return nil
}

// readEvent reads one 24-byte input_event; returns the triple and bytes read.
func readEvent(fd int) (evType, code uint16, value int32, n int, err error) {
	var ev [24]byte
	r, _, errno := syscall.Syscall6(syscall.SYS_READ, uintptr(fd),
		uintptr(unsafe.Pointer(&ev[0])), uintptr(len(ev)), 0, 0, 0)
	if errno != 0 {
		return 0, 0, 0, 0, errno
	}
	n = int(r)
	if n == 0 {
		return 0, 0, 0, 0, nil
	}
	if n != len(ev) {
		return 0, 0, 0, n, fmt.Errorf("short read: %d bytes", n)
	}
	evType = binary.LittleEndian.Uint16(ev[8:10])
	code = binary.LittleEndian.Uint16(ev[10:12])
	value = int32(binary.LittleEndian.Uint32(ev[12:16]))
	return
}

// findNode matches the device's input_id against /dev/input/event*/device/inputid.
func findNode(bus, vendor, product, version uint16) string {
	hex := func(v uint16) string { return fmt.Sprintf("%04x", v) }
	matches := func(id []byte) bool {
		f := strings.Fields(string(id))
		return len(f) == 4 && f[0] == hex(bus) && f[1] == hex(vendor) &&
			f[2] == hex(product) && f[3] == hex(version)
	}
	m, _ := filepath.Glob("/dev/input/event*")
	for _, node := range m {
		if id, err := os.ReadFile(filepath.Join(node, "device", "inputid")); err == nil && matches(id) {
			return node
		}
	}
	return ""
}
