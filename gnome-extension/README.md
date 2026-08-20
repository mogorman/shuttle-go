# Shuttle Pro (GNOME extension)

A tiny GNOME Shell extension that exposes shell state over the session D-Bus.
It exists so that tools running outside the shell (like `shuttle-go`) can read
information on Wayland that the shell otherwise keeps to itself. A single
`Info` method returns one JSON object with:

* the **focused window**'s title, `wm_class`, frame position, and monitor, and
* the **current pointer position**.

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

Query the current shell state:

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/ShuttlePro \
    org.gnome.Shell.Extensions.ShuttlePro.Info
```

The reply is a single JSON object:

```
{"title":"Lightworks","wm_class":"lightworks","x":10,"y":50,"monitor":0,"pointer_x":100,"pointer_y":300}
```

* `title` / `wm_class` — the focused window's title and WM_CLASS.
* `x` / `y` — the focused window's frame origin, in the global (all-monitors)
  coordinate space.
* `monitor` — the index of the display the focused window is on.
* `pointer_x` / `pointer_y` — the current pointer position.

When no window is focused the window fields (`title`, `wm_class`, `x`, `y`,
`monitor`) are `null`; the pointer fields are always present.

Pipe straight into `jq` to pretty-print or pick out a field:

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/ShuttlePro \
    org.gnome.Shell.Extensions.ShuttlePro.Info \
  | jq .
```

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/ShuttlePro \
    org.gnome.Shell.Extensions.ShuttlePro.Info \
  | jq '.title'
```

## Use in shuttle-go

* The absolute `/mousemove <x> <y> true` macro needs the current pointer
  position to compute the relative delta. On Wayland, `shuttle-go` reads
  `pointer_x` / `pointer_y` from this extension instead of the unreliable
  `/dev/input/mice` peek.
* Window-title / `wm_class` matching reads the focused window from this
  extension over D-Bus.
