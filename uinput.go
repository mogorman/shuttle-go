package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// input event types (linux/input.h)
const (
	evSync = 0x00
	evKey  = 0x01
	evRel  = 0x02
)

// EV_KEY button codes (linux/input-event-codes.h)
const (
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
	btnForward = 0x117
	btnBack   = 0x118
	btnSide   = 0x119
	btnExtra  = 0x11a
)

// EV_REL axis codes
const (
	relX = 0x00
	relY = 0x01
)

// /dev/uinput ioctls (linux/uinput.h)
const (
	uinputCreate  = 0xcf00
	uinputDestroy = 0xcf01
)

// mouseButtonCodes maps a /click button name (symbolic or hex) to an EV_KEY
// button code.
var mouseButtonCodes = map[string]int{
	"left":    btnLeft,
	"right":   btnRight,
	"middle":  btnMiddle,
	"forward": btnForward,
	"back":    btnBack,
	"side":    btnSide,
	"extr":    btnExtra,
	"0x00":    btnLeft,
	"0x01":    btnRight,
	"0x02":    btnMiddle,
	"0x03":    btnSide,
	"0x04":    btnExtra,
	"0x05":    btnForward,
	"0x06":    btnBack,
}

// uinputDevice is a virtual evdev input device created through /dev/uinput.
// It emits keyboard, relative-mouse and button events directly from this
// process, replacing the ydotool/ydotoold pair.
type uinputDevice struct {
	file *os.File
	fd   int
}

// uinputCreateArg mirrors struct uinput_setup from linux/uinput.h. The
// pointer fields are set to stack-allocated bitmaps that must outlive the
// ioctl call.
type uinputCreateArg struct {
	id      *inputID
	name    *byte
	ver     uint32
	evbits  *uint32
	keybits *byte
	relbits *uint32
	absbits *uint32
	abs     unsafe.Pointer
}

type inputID struct {
	bustype, vendor, product, version uint16
}

// newUinputDevice opens /dev/uinput and creates a virtual device named
// "shuttle-go-virtual" with EV_KEY (0-255), EV_REL (X, Y) and EV_SYN.
func newUinputDevice() (*uinputDevice, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("opening /dev/uinput: %s", err)
	}
	d := &uinputDevice{file: f, fd: int(f.Fd())}

	name := [32]byte{}
	copy(name[:], "shuttle-go-virtual")
	id := inputID{0x3, 0x0001, 0x0001, 0x0100}
	var evBits uint32
	evBits |= 1 << evSync
	evBits |= 1 << evKey
	evBits |= 1 << evRel
	keyBits := new([0x300]byte) // KEY_MAX is 0x2ff
	for code := 0; code <= 0x2ff; code++ {
		keyBits[code/8] |= 1 << uint(code%8)
	}
	var relBits uint32
	relBits |= 1 << relX
	relBits |= 1 << relY

	arg := uinputCreateArg{
		id:      &id,
		name:    (*byte)(unsafe.Pointer(&name[0])),
		ver:     0x0100,
		evbits:  &evBits,
		keybits: (*byte)(unsafe.Pointer(keyBits)),
		relbits: &relBits,
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(d.fd), uintptr(uinputCreate), uintptr(unsafe.Pointer(&arg))); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("UINPUT_CREATE: %s", errno)
	}

	return d, nil
}

// writeEvent writes a single 24-byte input_event to the device.
func (d *uinputDevice) writeEvent(evType, code uint16, value int32) error {
	ev := struct {
		tvSec  uint32
		tvUsec uint32
		typ    uint16
		code   uint16
		value  int32
	}{0, 0, evType, code, value}
	buf := (*[24]byte)(unsafe.Pointer(&ev))[:]
	if _, err := d.file.Write(buf); err != nil {
		return fmt.Errorf("writing input event: %s", err)
	}
	return nil
}

func (d *uinputDevice) syn() error {
	return d.writeEvent(evSync, 0, 0)
}

// KeyTap presses and releases each keycode in order, with a SYN after each
// press and after each release.
func (d *uinputDevice) KeyTap(codes []int) error {
	for _, code := range codes {
		if err := d.writeEvent(evKey, uint16(code), 1); err != nil {
			return err
		}
		if err := d.syn(); err != nil {
			return err
		}
	}
	for i := len(codes) - 1; i >= 0; i-- {
		if err := d.writeEvent(evKey, uint16(codes[i]), 0); err != nil {
			return err
		}
		if err := d.syn(); err != nil {
			return err
		}
	}
	return nil
}

// KeyHold presses each keycode (value 1) and SYNs.
func (d *uinputDevice) KeyHold(codes []int) error {
	for _, code := range codes {
		if err := d.writeEvent(evKey, uint16(code), 1); err != nil {
			return err
		}
	}
	return d.syn()
}

// KeyRelease releases each keycode (value 0) and SYNs.
func (d *uinputDevice) KeyRelease(codes []int) error {
	for _, code := range codes {
		if err := d.writeEvent(evKey, uint16(code), 0); err != nil {
			return err
		}
	}
	return d.syn()
}

// Type emits each rune of text as a tap.
func (d *uinputDevice) Type(text string) error {
	for _, r := range text {
		code, err := runeKeyCode(r)
		if err != nil {
			return err
		}
		if err := d.KeyTap([]int{code}); err != nil {
			return err
		}
	}
	return nil
}

// MouseMove moves the pointer by (dx, dy) relative units, or to the
// absolute position (x, y) when abs is true.
func (d *uinputDevice) MouseMove(dx, dy int, abs bool) error {
	if abs {
		x, y, err := d.currentMousePos()
		if err != nil {
			return err
		}
		dx = x + dx
		dy = y + dy
	}
	if err := d.writeEvent(evRel, relX, int32(dx)); err != nil {
		return err
	}
	if err := d.writeEvent(evRel, relY, int32(dy)); err != nil {
		return err
	}
	return d.syn()
}

// Click presses and releases the given EV_KEY button code repeats times.
func (d *uinputDevice) Click(code int, repeats int) error {
	if repeats < 1 {
		repeats = 1
	}
	for i := 0; i < repeats; i++ {
		if err := d.KeyTap([]int{code}); err != nil {
			return err
		}
	}
	return nil
}

// currentMousePos reads the current pointer position from /dev/input/mice
// (a 7-byte mouseevent: 3 status bytes + x, y, z as int16 little-endian).
func (d *uinputDevice) currentMousePos() (x, y int, err error) {
	f, err := os.Open("/dev/input/mice")
	if err != nil {
		return 0, 0, fmt.Errorf("opening /dev/input/mice: %s", err)
	}
	defer f.Close()

	buf := make([]byte, 7)
	n, err := f.Read(buf)
	if err != nil {
		return 0, 0, fmt.Errorf("reading /dev/input/mice: %s", err)
	}
	if n < 7 {
		return 0, 0, fmt.Errorf("short read from /dev/input/mice: %d bytes", n)
	}
	x = int(int16(binary.LittleEndian.Uint16(buf[3:5])))
	y = int(int16(binary.LittleEndian.Uint16(buf[5:7])))
	return
}

// Destroy sends UINPUT_DESTROY and closes the device.
func (d *uinputDevice) Destroy() {
	if d.file == nil {
		return
	}
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(d.fd), uintptr(uinputDestroy), 0)
	d.file.Close()
	d.file = nil
}
