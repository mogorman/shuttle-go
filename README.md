Linux driver for Contour Design Shuttle Pro V2
==============================================

## About this fork

This is a fork of [abourget/shuttle-go](https://github.com/abourget/shuttle-go)
focused on **better support for GNOME on Wayland**. The upstream project was
built around X11, where it could read the active window title directly and
inject keystrokes with `xdotool`. Neither of those works on Wayland, so this
fork:

* **Reads the active window and the pointer position over D-Bus** via the
  bundled [Shuttle Pro](gnome-extension/) GNOME extension (modeled on
  [window-calls](https://github.com/ickyicky/window-calls)), instead of the X11
  `_NET_ACTIVE_WINDOW` property.
* **Matches on `wm_class`** as well as the window title (see
  [Window matching](#window-matching)), so bindings can target a specific
  application window even when titles are generic.
* **Injects keys directly through a virtual `uinput` device** (the default
  driver) instead of `xdotool`, which works on both X11 and Wayland. No
  external daemon is required. It supports [macro sequences](#macro-sequences)
  (`/type`, `/sleep`, `/exec`, `/mousemove`, `/click`) for richer bindings.
* **Disables the Shuttle's pointer via `libinput`** on Wayland (and `xinput`
  on X11), so the device doesn't move the cursor.
* **Keeps running** across plug/unplug: it waits for the device, and recreates
  the virtual `uinput` device when the Shuttle is reconnected.

The rest of this document describes the (shared) configuration format and
behavior.

## Overview

The goal of this project is to use the Shuttle Pro V2 with the
Lightworks Non-Linear Video Editor, but `shuttle-go` allows you
to control anything.  It has support for:

* Sending keyboard events (with the default `uinput` driver)
* Sending Open Source Control messages (with the `ocs://` driver)
* Executing any command through `bash -c` (with the `exec` driver)

This program supports having **modifiers** for your Shuttle Pro V2
buttons.  So you can multiple the functionality of your buttons.  For
example, you can have different bindings for
<kbd>B1</kbd>+<kbd>F1</kbd> and <kbd>F1</kbd>.

## Window matching

Each app in the configuration is selected by matching the active window.
You can match on the window **title** (with `match_window_titles`) and/or
the window's **WM_CLASS** (with `match_wm_class`). Each list holds regular
expressions; a window matching *any* regexp in a list satisfies that
dimension.

- If only one list is present, that dimension alone decides the match.
- If **both** lists are present, **both** must match (AND). This lets you
  target one specific window but not all of them — e.g. a particular Emacs
  buffer while ignoring the rest.

```json
{
    "name": "Emacs - org mode",
    "match_wm_class": ["^Emacs$"],
    "match_window_titles": ["\\*org/.*\\.org\\*"],
    "bindings": { ... }
}
```

Matching on `wm_class` is often more stable than matching on the title,
since the class is set by the application and doesn't change with the
working directory or document name. On Wayland/GNOME the class is read from
the Shuttle Pro extension (see below); on X11 it is read from the `WM_CLASS`
window property.

## Layout

Buttons layout on the Contour Design Shuttle Pro v2:


```

           F1   F2   F3   F4

        F5   F6   F7   F8   F9


                (Shuttle)
        S-7 .. S-1  S0  S1 .. S7

     M1        JogL    JogR        M2



              B2        B3
            B1            B4

```
#### N.B. Contour Design Shuttle Pro v1 has the same buttons layout 


### Slow Jog

In addition to `JogL` and `JogR`, you can define bindings for
`SlowJogL` and `SlowJogR`. For example, you can use a slow jog use to
nudge by one frame at a time.

If you wish to not use slow jog, set the `slow_jog` key to `0` in the
configuration for this app. Otherwise, `slow_jog` represents the
minimum number of milliseconds between two events to be considered
slow. It defaults to 200 ms.


### Lightworks

Avoid Lightworks key bindings with modifiers however. Capital
letters are great as they cannot be combined, and are more direct and
they are less likely to conflict with your other bindings and
Lightworks recognizes them.

### Drivers

See `sample_config.json` for example configuration of each driver.

#### `uinput` (default)

The key names to use in the bindings are found here:
https://www.cl.cam.ac.uk/~mgk25/ucs/keysymdef.h or you can view them
locally in `/usr/include/X11/keysymdef.h` (stripped of the `XK_`
prefix).

This driver emits events through a virtual `uinput` device created by
`shuttle-go` itself (named `shuttle-go-virtual`), so no external tool or
daemon is needed. It must run with permission to open `/dev/uinput`
(typically as root, e.g. via `sudo` or a udev rule).

##### Macro sequences

With the `uinput` driver, a binding value can also be a **JSON array of
macro commands** instead of a single key. Nothing is held or released on
key-up; the difference is in how often the chain runs while the button is held:

* **Default** — the chain runs immediately on key-down, then repeats every
  25 ms for as long as the button stays held (like a key auto-repeat).
* **`"/once"`** — if the *first* command in the chain is `"/once"`, the chain
  runs exactly once on key-down and does not repeat. `"/once"` is a marker
  only; it is not itself executed.

Available macros:

* `"/once"` — (first position only) make the chain a one-shot
* `"/type <text>"` — types `<text>` through the virtual keyboard
* `"/sleep <seconds>"` — pauses for `<seconds>` seconds (e.g. `1.5`)
* `"/exec <command>"` — runs `<command>` through `bash -c`
* `"/mousemove <x> <y> [absolute]"` — moves the mouse by `<x> <y>`
  relative units; the optional third argument is `true`/`false`, and when
  `true` the move is made absolute (the current pointer position is read
  from `/dev/input/mice` first)
* `"/click <button> [repeats]"` — clicks a mouse button. `<button>` is a
  name: `left`, `right`, `middle`, `side`, `extr`, `forward`, `back` (or the
  corresponding hex codes `0x00`-`0x06`). The optional `repeats` count
  repeats the click (e.g. `"/click left 4"` clicks four times).

Example (repeats while held):

```json
"F5": [
    "/type hello world",
    "/sleep 1.5",
    "/type !!!",
    "/exec touch /tmp/shuttle-file",
    "/mousemove 500 300 true",
    "/click left"
]
```

Example (one-shot, via a leading `"/once"`):

```json
"F6": [
    "/once",
    "/type hello world",
    "/click left"
]
```

A plain string value (e.g. `"F1": "Escape"`) keeps the usual single-key
behavior, so the two forms can be mixed freely within one app.

#### `exec`

Any bindings triggered will execute the corresponding command through
`/bin/bash -c "your command"`

#### `osc://host:port`

In the configuration, use `"driver": "osc://host:port"`, then all your
bindings can be of the format: `/osc/address/path param1 param2
param3`.

You can send multiple messages with one key by separating those
bindings by ` + ` (that's a space, a plus sign, and another space).

A special `/sleep 0.123` message can be added, and it interpreted by
`shuttle-go` as a sleep between two OSC messages. Use that if your
program goes berzerk when messages are too close.


## Run

With:

    sudo shuttle-go /dev/input/by-id/usb-Contour_Design_ShuttlePRO_v2-event-mouse

### Wayland / GNOME

On Wayland (e.g. GNOME), the active window and the pointer position are not
exposed to other applications by default. To let `shuttle-go` read them,
install and enable the bundled [Shuttle Pro](gnome-extension/) GNOME extension,
which exposes both over D-Bus:

    make -C gnome-extension
    gnome-extensions install gnome-extension/shuttle-pro@shuttle-go.dev-v1.0.tar.gz
    gnome-extensions enable shuttle-pro@shuttle-go.dev

(See [gnome-extension/README.md](gnome-extension/README.md) for details.) This
makes the window title, `wm_class`, and pointer position available over D-Bus,
which is how `shuttle-go` reads them.

### For ShuttlePRO_v1
    shuttle-go /dev/input/by-id/usb-Contour_Design_ShuttlePRO-event-mouse

#### N.B. Running shuttle-go as sudo will cause shuttle-go to look for a valid config file in 

    /root/.shuttle-go.json

#### Without sudo, shuttle-go will look for a valid config file in the current user's home dir

    ~/.shuttle-go.json 
        

## Install in `udev` with:

**As root**, write file `/etc/udev/rules.d/01-shuttle-go.rules` with contents:

    ACTION=="add", ATTRS{name}=="Contour Design ShuttlePRO v2", MODE="0644"
    ACTION=="remove", ATTRS{name}=="Contour Design ShuttlePRO v2", RUN+="/usr/bin/pkill shuttle-go"

### For ShuttlePRO_v1

    ACTION=="add", ATTRS{name}=="Contour Design ShuttlePRO", MODE="0644"
    ACTION=="remove", ATTRS{name}=="Contour Design ShuttlePRO", RUN+="/usr/bin/pkill shuttle-go"

Then run, as **root**:

    udevadm control --reload-rules && udevadm trigger

From that point on, plug in the device, and run `shuttle-go` in any terminal (provided `shuttle-go` is in your `$PATH`).


## License

MIT

## Configuration reloading

`shuttle-go` watches the config file (the one given via `--config`, default
`~/.shuttle-go.json`) and **reloads it automatically when it changes** — no
restart needed. It uses `inotify` for immediate notification on Linux, and falls
back to polling every 5 seconds if `inotify` is unavailable. If an edit is
invalid (e.g. a broken regexp or malformed JSON), the reload is skipped and the
previous configuration stays in effect until the file is fixed.

## TODO

* Have a default SlowJog configuration.

* Make it auto-run on plug, with `udev` rules like:

```
    ACTION=="add", ATTRS{name}=="Contour Design ShuttlePRO v2", ENV{MINOR}=="79", RUN+="/home/abourget/go/src/github.com/abourget/shuttle-go/udev-start.sh"
    ACTION=="remove", ATTRS{name}=="Contour Design ShuttlePRO v2", RUN+="/usr/bin/pkill shuttle-go"
```
