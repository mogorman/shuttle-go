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
make install    # gnome-extensions install --enable <tarball>
```

or, from the repository root:

```sh
gnome-extensions install --enable \
    gnome-extension/mouse-position@shuttle-go.dev-v1.0.tar.gz
```

## Usage

Query the pointer position:

```sh
dbus-send --session --print-reply=literal \
    --dest=org.gnome.Shell \
    /org/gnome/Shell/Extensions/Mouse \
    org.gnome.Shell.Extensions.Mouse.Position
```

Response (a single JSON object):

```
{"x":100,"y":300}
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
