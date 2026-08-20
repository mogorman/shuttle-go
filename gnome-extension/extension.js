/* extension.js
 *
 * Exposes shell state over the session D-Bus, so other tools (e.g.
 * shuttle-go) can read it on Wayland, where the shell is the only thing that
 * reliably knows it. It provides:
 *
 *   - the current pointer position (and which monitor it is on), and
 *   - the focused window's title, wm_class, position, and monitor.
 *
 * Modeled on ickyicky/window-calls: a D-Bus interface is exported on the
 * session bus and its methods are answered by methods of the same object.
 *
 * Usage:
 *   dbus-send --session --print-reply=literal \
 *       --dest=org.gnome.Shell \
 *       /org/gnome/Shell/Extensions/ShuttlePro \
 *       org.gnome.Shell.Extensions.ShuttlePro.Position
 *
 *   ->  {"x":100,"y":300,"monitor":0}
 *
 *   dbus-send --session --print-reply=literal \
 *       --dest=org.gnome.Shell \
 *       /org/gnome/Shell/Extensions/ShuttlePro \
 *       org.gnome.Shell.Extensions.ShuttlePro.FocusedWindow
 *
 *   ->  {"title":"...","wm_class":"...","x":10,"y":50,"monitor":0}
 *
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import Gio from 'gi://Gio';

const SHUTTLE_PRO_DBUS_IFACE = `
<node>
   <interface name="org.gnome.Shell.Extensions.ShuttlePro">
      <method name="Position">
         <arg type="s" direction="out" name="pos" />
      </method>
      <method name="FocusedWindow">
         <arg type="s" direction="out" name="win" />
      </method>
   </interface>
</node>`;


export default class Extension {
  enable() {
    this._dbus = Gio.DBusExportedObject.wrapJSObject(SHUTTLE_PRO_DBUS_IFACE, this);
    this._dbus.export(Gio.DBus.session, '/org/gnome/Shell/Extensions/ShuttlePro');
  }

  disable() {
    this._dbus.flush();
    this._dbus.unexport();
    delete this._dbus;
  }

  // Position returns the current pointer position as a JSON object
  // {"x": <int>, "y": <int>, "monitor": <int>}. The monitor is the index of
  // the display the pointer is currently on, so callers can resolve the
  // position against the right screen in a multi-monitor setup.
  Position() {
    const [x, y] = global.get_pointer();
    const monitor = global.get_display().get_current_monitor();
    return JSON.stringify({ x: x, y: y, monitor: monitor });
  }

  // FocusedWindow returns the currently focused window as a JSON object
  // {"title": <string>, "wm_class": <string>, "x": <int>, "y": <int>,
  // "monitor": <int>}. x/y are the window's frame origin in the global
  // (all-monitors) coordinate space, and monitor is the index of the display
  // the window is on. If no window is focused, the fields are returned empty
  // (null) rather than as an error, so callers can simply check for null.
  FocusedWindow() {
    const win = global.get_window_actors()
      .find(w => w.meta_window.has_focus());

    if (!win) {
      return JSON.stringify({
        title: null,
        wm_class: null,
        x: null,
        y: null,
        monitor: null,
      });
    }

    const mw = win.meta_window;
    const frame = mw.get_frame_rect();
    return JSON.stringify({
      title: mw.get_title(),
      wm_class: mw.get_wm_class(),
      x: frame.x,
      y: frame.y,
      monitor: mw.get_monitor(),
    });
  }
}
