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

// EVIOCGBIT(ev, len) — _IOC(_IOC_READ, 'E', 0x20+ev, len). Used to query the
// set of event types/codes a device advertises.
func evIOCGBit(ev, len int) uintptr {
	return _ioc(_iocRead, 'E', 0x20+ev, len)
}

// _ioc encodes an input-subsystem ioctl number the way linux/_ioctl.h does:
//   _IOC(dir, type, nr, size) = (dir<<30)|(type<<8)|(size<<16)|nr
// where `type` is the full type char ('E' = 0x45), `nr` is the 8-bit command
// number (low byte), and `size` is the 16-bit length. Verified against the
// kernel's EVIOCGBIT values (0x80204520, 0x80204521, ...).
func _ioc(dir, typ, nr, size int) uintptr {
	return uintptr(uint(dir)<<30 | uint(typ)<<8 | uint(size)<<16 | uint(nr)&0xff)
}

const (
	_iocRead = 2 // _IOC_READ
	evMax    = 0x20
)

func evTypeName(t int) string {
	switch t {
	case 0:
		return "EV_SYN"
	case 1:
		return "EV_KEY"
	case 2:
		return "EV_REL"
	case 3:
		return "EV_ABS"
	default:
		return fmt.Sprintf("EV_%d", t)
	}
}

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
// process, replacing the ydotool/ydotoold pair.
type uinputDevice struct {
	file    *os.File
	fd      int
	node    string // /dev/input/eventN node for the created device ("" if unknown)
	readF   *os.File
	bustype uint16
	vendor  uint16
	product uint16
	version uint16
}

// uinputNode finds the /dev/input/eventN node for the created device by
// matching each event's sysfs input_id (bus/vendor/product/version) against the
// id we programmed into the uinput device. Matching on input_id is robust
// across kernels, unlike the name string (whose sysfs encoding varies).
func (d *uinputDevice) uinputNode() string {
	nodes := globDevInputEvents()
	if *debugMode {
		fmt.Printf("uinput node scan: %d /dev/input/event* nodes\n", len(nodes))
	}
	for _, node := range nodes {
		idPath := filepath.Join(node, "device", "inputid")
		id, err := os.ReadFile(idPath)
		if err != nil {
			if *debugMode {
				fmt.Printf("uinput node scan: %s: %s\n", idPath, err)
			}
			continue
		}
		// inputid is "bus vendor product version", each 4 hex digits.
		f := strings.Fields(string(id))
		if len(f) != 4 {
			if *debugMode {
				fmt.Printf("uinput node scan: %s: unexpected fields %q\n", idPath, string(id))
			}
			continue
		}
		if f[0] == hex16(d.bustype) && f[1] == hex16(d.vendor) &&
			f[2] == hex16(d.product) && f[3] == hex16(d.version) {
			return node
		}
	}
	return ""
}

func hex16(v uint16) string { return fmt.Sprintf("%04x", v) }

func globDevInputEvents() []string {
	m, _ := filepath.Glob("/dev/input/event*")
	return m
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
// "shuttle-go-virtual" with EV_KEY (0-KEY_MAX) and EV_REL (X, Y).
//
// The protocol: write a uinput_user_dev blob to the fd, then issue the
// no-argument UI_DEV_CREATE ioctl. The kernel derives the supported event
// bits from the id/version fields and the (zeroed) abs arrays.
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
	d.bustype, d.vendor, d.product, d.version = dev.bustype, dev.vendor, dev.product, dev.version

	if _, err := f.Write((*[unsafe.Sizeof(dev)]byte)(unsafe.Pointer(&dev))[:]); err != nil {
		f.Close()
		return nil, fmt.Errorf("writing uinput_user_dev: %s", err)
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(d.fd), uintptr(uiDevCreate), 0); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("UI_DEV_CREATE: %s", errno)
	}

	d.node = d.uinputNode()
	if *debugMode {
		fmt.Println("uinput device node:", d.node)
	}

	// Confirm the device actually advertises the event types we programmed,
	// independent of any node discovery. EVIOCGBIT(0, size) returns a bitmap of
	// supported event types; the buffer must be exactly `size` bytes.
	var evbits [evMax]byte
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(d.fd),
		uintptr(evIOCGBit(0, evMax)), uintptr(unsafe.Pointer(&evbits[0]))); errno == 0 {
		types := make([]string, 0, 8)
		for t := 0; t < evMax; t++ {
			if evbits[t/8]&(1<<uint(t%8)) != 0 {
				types = append(types, evTypeName(t))
			}
		}
		if *debugMode {
			fmt.Println("uinput device advertises event types:", strings.Join(types, ","))
		}
	} else if *debugMode {
		fmt.Println("uinput EVIOCGBIT failed:", errno)
	}

	return d, nil
}

// openReadNode opens the device's own /dev/input/eventN node (if it exists)
// so emitted events can be read back for a self-test. If the node was not
// resolved at creation time, it re-scans for it.
func (d *uinputDevice) openReadNode() error {
	if d.node == "" {
		d.node = d.uinputNode()
	}
	if d.node == "" {
		return fmt.Errorf("no /dev/input node for the uinput device")
	}
	f, err := os.OpenFile(d.node, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("opening %s: %s", d.node, err)
	}
	d.readF = f
	return nil
}

// ReadBack reads and returns the next raw input_event from the device's own
// /dev/input node, for verifying that a written event actually reached the
// device. It returns the (type, code, value) triple and whether a SYN_REPORT
// followed.
func (d *uinputDevice) ReadBack() (evType, code uint16, value int32, ok bool, err error) {
	if d.readF == nil {
		return 0, 0, 0, false, fmt.Errorf("read node not open")
	}
	var ev [24]byte
	n, _, errno := syscall.Syscall6(syscall.SYS_READ, uintptr(d.readF.Fd()),
		uintptr(unsafe.Pointer(&ev[0])), uintptr(len(ev)), 0, 0, 0)
	if errno != 0 {
		return 0, 0, 0, false, fmt.Errorf("reading %s: %s", d.node, errno)
	}
	if n != uintptr(len(ev)) {
		return 0, 0, 0, false, fmt.Errorf("short read: %d bytes", n)
	}
	evType = binary.LittleEndian.Uint16(ev[8:10])
	code = binary.LittleEndian.Uint16(ev[10:12])
	value = int32(binary.LittleEndian.Uint32(ev[12:16]))
	return evType, code, value, evType == evSync, nil
}

// writeEvent writes a single 24-byte input_event to the device using the raw
// write(2) syscall (bypassing os.File, which would be fine but is slower and
// obscures the errno). The 24-byte struct input_event layout is:
//
//	__kernel_time_t t_sec;   // u32
//	__s32             t_usec;
//	enum  input_event_type  type;   // u16
//	enum  input_event_code  code;   // u16
//	__s32             value;
//
// which is 4+4+2+2+4 = 16 bytes on a 32-bit time_t, but the on-disk layout
// pads t_sec to 8 bytes on 64-bit kernels, giving 8+4+2+2+4 = 20 bytes. We
// emit the 24-byte form the kernel's evdev read() expects (8-byte time_t,
// 4-byte usec, 2+2 type/code, 4-byte value, 4 bytes padding) so a single
// write() delivers exactly one event.
func (d *uinputDevice) writeEvent(evType, code uint16, value int32) error {
	var ev [24]byte
	binary.LittleEndian.PutUint32(ev[0:4], 0)    // t_sec
	binary.LittleEndian.PutUint32(ev[4:8], 0)    // t_usec
	binary.LittleEndian.PutUint16(ev[8:10], evType)
	binary.LittleEndian.PutUint16(ev[10:12], code)
	binary.LittleEndian.PutUint32(ev[12:16], uint32(value))
	// ev[16:24] is zero padding.

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
