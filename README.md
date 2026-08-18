Linux driver for Contour Design Shuttle Pro V2
==============================================

The goal of this project is to use the Shuttle Pro V2 with the
Lightworks Non-Linear Video Editor, but `shuttle-go` allows you
to control anything.  It has support for:

* Sending keyboard events (with the default `ydotool` driver)
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
the `window-calls` extension (see below); on X11 it is read from the
`WM_CLASS` window property.

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

#### `ydotool` (default)

The key names to use in the bindings are found here:
https://www.cl.cam.ac.uk/~mgk25/ucs/keysymdef.h or you can view them
locally in `/usr/include/X11/keysymdef.h` (stripped of the `XK_`
prefix).

You need to install the `ydotool` package before using this driver (default).

##### Macro sequences

With the `ydotool` driver, a binding value can also be a **JSON array of
macro commands** instead of a single key. The sequence runs top-to-bottom on
the initial key press (it is a one-shot; nothing is held or released on key-up).

Available macros:

* `"/type <text>"` — types `<text>` via `ydotool type`
* `"/sleep <seconds>"` — pauses for `<seconds>` seconds (e.g. `1.5`)
* `"/exec <command>"` — runs `<command>` through `bash -c`
* `"/mousemove <x> <y> [absolute]"` — moves the mouse to `<x> <y>` via
  `ydotool mousemove`; the optional third argument is `true`/`false`, and
  when `true` the `-a` (absolute) flag is added
* `"/click <button> [repeats]"` — clicks a mouse button via
  `ydotool click <code>`. `<button>` is a name or a hex code: `left`/`0x00`,
  `right`/`0x01`, `middle`/`0x02`, `side`/`0x03`, `extr`/`0x04`,
  `forward`/`0x05`, `back`/`0x06`, `task`/`0x07`, `mousedown`/`0x40`,
  `mouseup`/`0x80`. The optional `repeats` count adds `-r <n>` (e.g.
  `"/click left 4"` → `ydotool click 0x00 -r 4`).

Example:

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

On Wayland (e.g. GNOME), window titles are not exposed to other applications by
default. To allow `shuttle-go` to see the active window title, install the
[window-calls](https://github.com/ickyicky/window-calls) extension:

    gnome-extensions install https://github.com/ickyicky/window-calls

This makes the window title and `wm_class` available over D-Bus, which is
how `shuttle-go` reads them.

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

## TODO

* Don't require `ydotool`
  * Use xgb's `xtest` package and send the FakeInput directly there..

* Watch the configuration file, and reload on change.

* Have a default SlowJog configuration.

* Make it auto-run on plug, with `udev` rules like:

```
    ACTION=="add", ATTRS{name}=="Contour Design ShuttlePRO v2", ENV{MINOR}=="79", RUN+="/home/abourget/go/src/github.com/abourget/shuttle-go/udev-start.sh"
    ACTION=="remove", ATTRS{name}=="Contour Design ShuttlePRO v2", RUN+="/usr/bin/pkill shuttle-go"
```
