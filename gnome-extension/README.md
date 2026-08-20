# Shuttle Pro (GNOME extension)

A tiny GNOME Shell extension that exposes shell state over the session D-Bus.
It exists so that tools running outside the shell (like `shuttle-go`) can read
information on Wayland that the shell otherwise keeps to itself:

* the **current pointer position** (and which monitor it is on), and
* the **focused window**'s title, `wm_class`, position, and monitor.

It is modeled on
[window-calls](https://github.com/ickyicky/window-calls), which exposes window
information the same way.

## Build

```sh
make            # produces shuttle-pro@shuttle-go.dev-v1.0.tar.gz
```

## Install

```sh
make install    # gnome-extensions install <tarball>
```

or, from the repository root:

```sh
gnome-extensions install \
    gnome-extension/shuttle-pro@shuttle-go.dev-v1.0.tar.gz
```

## Activate

Installing does not enable the extension. Turn it on either in the
**Extensions** app (toggle **Shuttle Pro** on) or from the command line:

```sh
make enable    # gnome-extensions enable shuttle-pro@shuttle-go.dev
```

## Usage

### Pointer position

Query the pointer position:

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/ShuttlePro \
    org.gnome.Shell.Extensions.ShuttlePro.Position
```

Response (a single JSON object). `monitor` is the index of the display the
pointer is on, so the position can be resolved against the right screen in a
multi-monitor setup:

```
{"x":100,"y":300,"monitor":0}
```

### Focused window

Query the focused window's title, `wm_class`, frame origin, and monitor:

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/ShuttlePro \
    org.gnome.Shell.Extensions.ShuttlePro.FocusedWindow
```

Response (a single JSON object). `x`/`y` are the window's frame origin in the
global (all-monitors) coordinate space, and `monitor` is the index of the
display it is on. When no window is focused the fields are `null`:

```
{"title":"Lightworks","wm_class":"lightworks","x":10,"y":50,"monitor":0}
```

With `jq` (note the literal reply is wrapped in a D-Bus array by `dbus-send`):

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/ShuttlePro \
    org.gnome.Shell.Extensions.ShuttlePro.FocusedWindow \
  | sed -n 's/.*\(.*\)/\1/p' | jq .
```

## Use in shuttle-go

* The absolute `/mousemove <x> <y> true` macro needs the current pointer
  position to compute the relative delta. On Wayland, `shuttle-go` reads it
  from this extension instead of the unreliable `/dev/input/mice` peek.
* Window-title / `wm_class` matching reads the focused window from this
  extension over D-Bus.
