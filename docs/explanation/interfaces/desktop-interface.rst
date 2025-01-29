.. _exp_desktop_interface:

Desktop interface
=================

.. @artefact desktop interface

The desktop interface
provides access to the host system's display (Wayland/X11) socket(s)
from inside the workshop,
allowing it to securely run GUI applications.

By using the interface,
the SDK publisher allows the workshop to utilise the host's display
which can be useful for various SDK-specific tasks
such as building graphical applications or using editors without remote support.

The interface isn't connected automatically at launch and refresh
for security reasons.
The :command:`workshop connect` and :command:`workshop disconnect` commands
can be invoked manually after the workshop has started:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop connect ws/desktop-sdk:desktop
   $ workshop disconnect ws/desktop-sdk:desktop


Establishing a connection means
a proxy Unix domain socket has been created
and the following environment variables have been set:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 30 30 30

   * - Wayland
     - X11
     - Both

   * - | :envvar:`$WAYLAND_DISPLAY`
       | :envvar:`$XDG_SESSION_TYPE`
       | :envvar:`$QT_QPA_PLATFORM`
       | :envvar:`$ELECTRON_OZONE_PLATFORM_HINT`
       | :envvar:`$XDG_BACKEND`
     - | :envvar:`$DISPLAY`
       | :envvar:`$XDG_SESSION_TYPE`
       | :envvar:`$QT_QPA_PLATFORM`
       | :envvar:`$ELECTRON_OZONE_PLATFORM_HINT`
       | :envvar:`$XDG_BACKEND`
       | :envvar:`$XAUTHORITY`:sup:`*`
     - | :envvar:`$WAYLAND_DISPLAY`
       | :envvar:`$DISPLAY`
       | :envvar:`$XDG_SESSION_TYPE`
       | :envvar:`$QT_QPA_PLATFORM`
       | :envvar:`$ELECTRON_OZONE_PLATFORM_HINT`
       | :envvar:`$XDG_BACKEND`
       | :envvar:`$XAUTHORITY`:sup:`*`

*\*only set if present on the host*


To check if the interface is connected:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections --all

     Interface  Plug                   Slot                Notes
     ...
     desktop    ws/desktop-sdk:desktop ws/system:desktop   manual


This means the host's display socket (Wayland, X11 or both) is available inside the workshop:

.. @artefact workshop shell

.. code-block:: console

   $ workshop shell ws
   workshop@ws-8584e571$ ls $XDG_RUNTIME_DIR | grep wayland

     wayland-1

.. code-block:: console

   $ workshop shell ws
   workshop@ws-8584e571$ ls /tmp/.X11-unix

     X0

See also
--------

Explanation:

- :ref:`exp_interfaces`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdk_definition`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_shell`
