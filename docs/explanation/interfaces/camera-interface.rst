.. _exp_camera_interface:

Camera interface
================

.. @artefact camera interface

The camera interface
enables access to the host system's cameras
and other video capture devices inside the workshop.

By using the interface,
the SDK publisher allows the workshop to access the host's cameras,
which can be useful in various SDK-specific tasks
such as testing hardware or embedded devices.

.. _exp_camera_plug:

Camera interface plug
---------------------

An essential element here is the camera interface plug,
which is declared in the SDK definition.

A basic structure would include just the name of the plug itself
and the interface (:samp:`camera`).

Defining the plug in an SDK
allows the workshops using this SDK to connect to the host's USB-based cameras,
which can be useful in various SDK-specific tasks
such as testing hardware or embedded devices.


.. _exp_camera_slot:

Camera interface slot
---------------------

To let SDKs in a workshop access the host's cameras,
|ws_markup| provides a camera interface slot
that multiple camera interface plugs can access.

When the SDK is installed at run-time during launch and refresh operations,
|ws_markup| checks that the plug targeting the slot
passes :ref:`validation <exp_interfaces_validation>`;
if it does,
it can be connected.


Connection
----------

The interface isn't connected automatically at launch and refresh
for security reasons.
The :command:`workshop connect` and :command:`workshop disconnect` commands
can be invoked manually after the workshop has started:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop connect ws/camera-sdk:camera
   $ workshop disconnect ws/camera-sdk:camera


Establishing a connection means
that all currently connected USB-based cameras
will be made available inside the workshop.
New cameras can be added
by disconnecting and then reconnecting the camera interface.

To check if the interface is connected:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections --all

     Interface  Plug              Slot     Notes
     ...
     camera     ws/camera:camera  ws/system:camera  manual


This means the host's cameras are available inside the workshop:

.. @artefact workshop shell

.. code-block:: console

   $ workshop shell ws
   workshop@ws-8584e571$ ls /dev/video*

     /dev/video0  /dev/video1


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
