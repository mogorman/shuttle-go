/* extension.js
 *
 * Exposes shell state over the session D-Bus, so other tools (e.g.
 * shuttle-go) can read it on Wayland, where the shell is the only thing that
 * reliably knows it. A single Info method returns the focused window's
 * title, wm_class, frame position, and monitor, plus the current pointer
 * position.
 *
 * Modeled on ickyicky/window-calls: a D-Bus interface is exported on the
 * session bus and its methods are answered by methods of the same object.
 *
 * Usage:
 *   dbus-send --session --print-reply=literal \
 *       --dest=org.gnome.Shell \
 *       /org/gnome/Shell/Extensions/ShuttlePro \
 *       org.gnome.Shell.Extensions.ShuttlePro.Info \
 *     | jq .
 *
 *   ->  {"title":"Lightworks","wm_class":"lightworks","x":10,"y":50,
 *        "monitor":0,"pointer_x":100,"pointer_y":300}
 *
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import Gio from 'gi://Gio';

const SHUTTLE_PRO_DBUS_IFACE = `
<node>
   <interface name="org.gnome.Shell.Extensions.ShuttlePro">
      <method name="Info">
         <arg type="s" direction="out" name="info" />
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

  // Info returns the current shell state as a single JSON object:
  //
  //   {
  //     "title":     <string|null>,  focused window title
  //     "wm_class":  <string|null>,  focused window WM_CLASS
  //     "x":         <int|null>,     focused window frame origin x
  //     "y":         <int|null>,     focused window frame origin y
  //     "monitor":   <int|null>,     monitor index the window is on
  //     "pointer_x": <int>,          current pointer x
  //     "pointer_y": <int>          current pointer y
  //   }
  //
  // The window fields (title, wm_class, x, y, monitor) describe the focused
  // window; x/y are its frame origin in the global (all-monitors) coordinate
  // space, and monitor is the index of the display it is on. If no window is
  // focused those fields are null rather than an error, so callers can simply
  // check for null. The pointer position is always present.
  Info() {
    const [px, py] = global.get_pointer();

    const win = global.get_window_actors()
      .find(w => w.meta_window.has_focus());

    if (!win) {
      return JSON.stringify({
        title: null,
        wm_class: null,
        x: null,
        y: null,
        monitor: null,
        pointer_x: px,
        pointer_y: py,
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
      pointer_x: px,
      pointer_y: py,
    });
  }
}
