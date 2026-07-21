.. _exp_custom_device_interface:

.. meta::
   :description: Documentation on the custom-device interface that enables
                 workshops to access arbitrary host devices belonging to a
                 given subsystem, for hardware testing, device development, and
                 other applications that need access to non-standard devices.

Custom device interface
=======================

.. @artefact custom-device interface

The custom device interface
enables access to arbitrary host devices inside the workshop,
identified by the device *subsystem* they belong to
(for example, :samp:`input`, :samp:`tty`, or :samp:`usb`).

By using the interface,
the SDK publisher allows the workshop to access devices
that no dedicated interface covers,
such as serial adapters, input devices, or other peripherals
used for testing hardware or embedded devices.

.. _exp_custom_device_plug:

Custom device interface plug
----------------------------

An essential element here is the custom device interface plug,
which is declared in the SDK definition.

Its structure includes the name of the plug,
the interface (:samp:`custom-device`),
and the optional :samp:`subsystem`, :samp:`vendorid`,
and :samp:`productid` device filters.
At least one of these filters must be set.

Defining the plug in an SDK
allows the workshops using this SDK to connect to matching devices,
which can unlock additional SDK functionality.


.. _exp_custom_device_subsystem:

Device subsystems
-----------------

.. @artefact custom-device interface attributes

The :samp:`subsystem` attribute for a given device
is defined by the Linux kernel.
One way to query device properties is :command:`udevadm info`:

.. code-block:: console

   $ udevadm info --query=property --property=SUBSYSTEM /dev/input/event0

     SUBSYSTEM=input


.. _exp_custom_device_filters:

Product and vendor filters
--------------------------

When a subsystem matches more devices than wanted,
the optional :samp:`vendorid` and :samp:`productid` attributes
narrow the selection down to devices
reporting the given vendor and product ID.
Both are matched against the respective identifiers reported by the kernel,
which can be queried with :command:`udevadm info`:

.. code-block:: console

   $ udevadm info --query=property --property=ID_VENDOR_ID --property=ID_MODEL_ID /dev/ttyUSB0

     ID_VENDOR_ID=0403
     ID_MODEL_ID=6001

Because a product ID is only meaningful within a vendor's namespace,
setting :samp:`productid` also requires :samp:`vendorid`.


.. _exp_custom_device_slot:

Custom device interface slot
----------------------------

To let SDKs in a workshop access the host's devices,
|ws_markup| provides a custom device interface slot
that multiple custom device interface plugs can access.

When the SDK is installed at runtime during launch and refresh operations,
|ws_markup| checks that the plug targeting the slot
passes :ref:`validation <exp_interfaces_validation>`;
if it does,
it can be connected.


Connection
----------

The interface isn't connected automatically at launch and refresh
for security reasons.
The :command:`workshop connect` and :command:`workshop disconnect` commands
can be invoked manually after the workshop has started:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop connect ws/input-sdk:input-device :custom-device
   $ workshop disconnect ws/input-sdk:input-device


Establishing a connection means
that all existing host devices belonging to the plug's subsystem
will be made available inside the workshop.
While the connection is active,
adding new devices on the host will also make them available inside the workshop,
whereas unplugged devices will also be removed from the workshop.

To check if the interface is connected:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections --all

     INTERFACE      PLUG                       SLOT                     NOTES
     ...
     custom-device  ws/input-sdk:input-device  ws/system:custom-device  manual


This means the host's devices from the given subsystem
are available inside the workshop:

.. @artefact workshop shell

.. code-block:: console

   $ workshop shell ws
   workshop@ws:/project$ ls /dev/input/

     event0  event1  mice


See also
--------

Explanation:

- :ref:`exp_interface_concepts`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdk_definition`
- :ref:`exp_workshop_definition`


How-to guides:

- :ref:`how_use_host_devices`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_shell`
