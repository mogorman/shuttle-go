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
	btnLeft    = 0x110
	btnRight   = 0x111
	btnMiddle  = 0x112
	btnForward = 0x117
	btnBack    = 0x118
	btnSide    = 0x119
	btnExtra   = 0x11a
)

// EV_REL axis codes
const (
	relX = 0x00
	relY = 0x01
)

// /dev/uinput ioctls (linux/uinput.h). UI_DEV_CREATE and UI_DEV_DESTROY are
// plain _IO ioctls: they take no argument. The device is configured by
// writing a uinput_user_dev blob to the fd first.
const (
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
)

// uinput_user_dev layout constants (linux/uinput.h, linux/input.h).
const (
	uinputMaxNameSize = 80
	absCnt            = 0x40 // ABS_CNT
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
// process, replacing the ydotool/ydotoold pair. Events are written straight to
// the /dev/uinput fd (the same pattern used by github.com/bendahl/uinput);
// no /dev/input node discovery is needed.
type uinputDevice struct {
	file *os.File
	fd   int
}

// uinputUserDev mirrors struct uinput_user_dev from linux/uinput.h.
//
//	char name[UINPUT_MAX_NAME_SIZE];
//	struct input_id id;
//	__u32 ff_effects_max;
//	__s32 absmax[ABS_CNT];
//	__s32 absmin[ABS_CNT];
//	__s32 absfuzz[ABS_CNT];
//	__s32 absflat[ABS_CNT];
//
// The four abs arrays are zeroed (no absolute axes are declared), but they
// still occupy 4*64*4 = 1024 bytes, so the struct is 92 + 1024 = 1116 bytes.
// The kernel reads the whole struct, so all of it must be written.
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

// newUinputDevice opens /dev/uinput and creates a virtual device named
// "shuttle-go-virtual" with EV_KEY and EV_REL (X, Y) support.
//
// The protocol: write a uinput_user_dev blob to the fd, then issue the
// no-argument UI_DEV_CREATE ioctl. All subsequent events are written to the
// same fd.
func newUinputDevice() (*uinputDevice, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening /dev/uinput: %s", err)
	}
	d := &uinputDevice{file: f, fd: int(f.Fd())}

	var dev uinputUserDev
	copy(dev.name[:], "shuttle-go-virtual")
	dev.bustype = 0x03 // BUS_USB
	dev.vendor = 0x0001
	dev.product = 0x0001
	dev.version = 0x0100

	if _, err := f.Write((*[unsafe.Sizeof(dev)]byte)(unsafe.Pointer(&dev))[:]); err != nil {
		f.Close()
		return nil, fmt.Errorf("writing uinput_user_dev: %s", err)
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(d.fd), uintptr(uiDevCreate), 0); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("UI_DEV_CREATE: %s", errno)
	}

	return d, nil
}

// writeEvent writes a single 24-byte input_event to the device using the raw
// write(2) syscall. The 24-byte struct input_event layout is:
//
//	__kernel_time_t t_sec;   // u64 (8 bytes on 64-bit)
//	__s32             t_usec;
//	enum  input_event_type  type;   // u16
//	enum  input_event_code  code;   // u16
//	__s32             value;
//
// which is 8+4+2+2+4 = 20 bytes, padded to 24. A single write() delivers
// exactly one event.
func (d *uinputDevice) writeEvent(evType, code uint16, value int32) error {
	var ev [24]byte
	binary.LittleEndian.PutUint64(ev[0:8], 0) // t_sec
	binary.LittleEndian.PutUint32(ev[8:12], 0) // t_usec
	binary.LittleEndian.PutUint16(ev[12:14], evType)
	binary.LittleEndian.PutUint16(ev[14:16], code)
	binary.LittleEndian.PutUint32(ev[16:20], uint32(value))
	// ev[20:24] is zero padding.

	n, _, errno := syscall.Syscall6(syscall.SYS_WRITE, uintptr(d.fd),
		uintptr(unsafe.Pointer(&ev[0])), uintptr(len(ev)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("writing input event (type=%d code=%d value=%d): %s (wrote %d bytes)",
			evType, code, value, errno, n)
	}
	if n != uintptr(len(ev)) {
		return fmt.Errorf("short write: wrote %d of %d bytes", n, len(ev))
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

// Destroy sends UI_DEV_DESTROY and closes the device.
func (d *uinputDevice) Destroy() {
	if d.file == nil {
		return
	}
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(d.fd), uintptr(uiDevDestroy), 0)
	d.file.Close()
	d.file = nil
}
