# Mouse Position (GNOME extension)

A tiny GNOME Shell extension that exposes the **current pointer position** over
the session D-Bus. It exists so that tools running outside the shell (like
`shuttle-go`) can read the real cursor position on Wayland, where the pointer
is not otherwise visible to other processes.

It is modeled on
[window-calls](https://github.com/ickyicky/window-calls), which exposes window
information the same way.

## Build

```sh
make            # produces mouse-position@shuttle-go.dev-v1.0.tar.gz
```

## Install

```sh
make install    # gnome-extensions install <tarball>
```

or, from the repository root:

```sh
gnome-extensions install \
    gnome-extension/mouse-position@shuttle-go.dev-v1.0.tar.gz
```

## Activate

Installing does not enable the extension. Turn it on either in the
**Extensions** app (toggle **Mouse Position** on) or from the command line:

```sh
make enable    # gnome-extensions enable mouse-position@shuttle-go.dev
```

## Usage

Query the pointer position:

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/Mouse \
    org.gnome.Shell.Extensions.Mouse.Position
```

Response (a single JSON object). `monitor` is the index of the display the
pointer is on, so the position can be resolved against the right screen in a
multi-monitor setup:

```
{"x":100,"y":300,"monitor":0}
```

With `jq` (note the literal reply is wrapped in a D-Bus array by `dbus-send`):

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/Mouse \
    org.gnome.Shell.Extensions.Mouse.Position \
  | sed -n 's/.*\(.*\)/\1/p' | jq .
```

## Use in shuttle-go

The absolute `/mousemove <x> <y> true` macro needs the current pointer
position to compute the relative delta. On Wayland, `shuttle-go` can read it
from this extension instead of the unreliable `/dev/input/mice` peek.
