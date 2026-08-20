/* extension.js
 *
 * Exposes the current pointer position over the session D-Bus, so other
 * tools (e.g. shuttle-go's absolute /mousemove macro) can read it.
 *
 * Modeled on ickyicky/window-calls: a D-Bus interface is exported on the
 * session bus and its methods are answered by methods of the same object.
 *
 * Usage:
 *   dbus-send --session --print-reply=literal \
 *       --dest=org.gnome.Shell \
 *       /org/gnome/Shell/Extensions/Mouse \
 *       org.gnome.Shell.Extensions.Mouse.Position
 *
 *   ->  {"x":100,"y":300}
 *
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import Gio from 'gi://Gio';

const MOUSE_DBUS_IFACE = `
<node>
   <interface name="org.gnome.Shell.Extensions.Mouse">
      <method name="Position">
         <arg type="s" direction="out" name="pos" />
      </method>
   </interface>
</node>`;


export default class Extension {
  enable() {
    this._dbus = Gio.DBusExportedObject.wrapJSObject(MOUSE_DBUS_IFACE, this);
    this._dbus.export(Gio.DBus.session, '/org/gnome/Shell/Extensions/Mouse');
  }

  disable() {
    this._dbus.flush();
    this._dbus.unexport();
    delete this._dbus;
  }

  // Position returns the current pointer position as a JSON object
  // {"x": <int>, "y": <int>}.
  Position() {
    const [x, y] = global.get_pointer();
    return JSON.stringify({ x: x, y: y });
  }
}
