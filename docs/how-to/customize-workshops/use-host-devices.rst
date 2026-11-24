.. _how_use_host_devices:

.. meta::
   :description: How-to guide on accessing host devices inside a workshop with
                 the custom device interface, covering subsystem and device ID
                 discovery, declaring a plug in an SDK, connecting the
                 interface manually, and reaching the devices inside the
                 workshop.

How to use host devices
=======================

.. @tests in tests/docs-how-to/use-host-devices/task.yaml

|ws_markup| exposes arbitrary host devices to a workshop
through the custom device interface,
identified by the device *subsystem* they belong to
(for example, :samp:`input`, :samp:`tty`, or :samp:`usb`)
and, when needed, narrowed by vendor and product identifiers.
A plug declared on an SDK sets at least one of
:samp:`subsystem`, :samp:`vendorid`, or :samp:`productid`;
|ws_markup| then passes matching host devices
into the workshop once the plug is connected.

The interface is never connected automatically,
so reaching a host device follows a short sequence:
find the device attributes, declare a plug, connect the interface,
then use the devices inside the workshop.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- A host device you want to expose to the workshop,
  such as an input device at :file:`/dev/input/event0`.


Identify the device attributes
------------------------------

Every device node on the host belongs to a subsystem
defined by the Linux kernel.
Query the subsystem of a device with :command:`udevadm info`:

.. code-block:: console

   $ udevadm info --query=property --property=SUBSYSTEM /dev/input/event0

     SUBSYSTEM=input


The device at :file:`/dev/input/event0` belongs to the :samp:`input` subsystem.
That name is what the plug declares in the next step.

If the subsystem covers more devices than the workshop needs,
and the device reports vendor and model properties,
query those properties too:

.. code-block:: console

   $ udevadm info --query=property \
       --property=SUBSYSTEM \
       --property=ID_VENDOR_ID \
       --property=ID_MODEL_ID \
       /dev/ttyUSB0

     SUBSYSTEM=tty
     ID_VENDOR_ID=0403
     ID_MODEL_ID=6001


The optional :samp:`vendorid` attribute matches :samp:`ID_VENDOR_ID`.
The optional :samp:`productid` attribute matches :samp:`ID_MODEL_ID`;
use :samp:`productid` for the device or product identifier
reported by :command:`udevadm`.


Declare a custom device plug
----------------------------

A custom device plug lives on an SDK, not on the workshop directly.
To add one without publishing a separate SDK,
declare an in-project SDK:
a small definition stored under :file:`.workshop/`
that the workshop references with the :samp:`project-` prefix.

Give the SDK a name and declare the plug,
naming the subsystem from the previous step:

.. code-block:: yaml
   :caption: .workshop/input-sdk/sdk.yaml

   name: input-sdk
   plugs:
     input-device:
       interface: custom-device
       subsystem: input


If you want only one device model from a subsystem,
add the vendor and product identifiers too:

.. code-block:: yaml
   :caption: .workshop/serial-sdk/sdk.yaml

   name: serial-sdk
   plugs:
     serial-adapter:
       interface: custom-device
       subsystem: tty
       vendorid: "0403"
       productid: "6001"


.. warning::

   Avoid using :samp:`subsystem: tty` by itself for serial adapters.
   It can match broad host TTY devices that the workshop already provides,
   such as :file:`/dev/console`, :file:`/dev/tty`,
   and :file:`/dev/ptmx`,
   and make the connection fail.
   For :samp:`tty` devices,
   add :samp:`vendorid`
   and, when available, :samp:`productid`
   to target the specific device model.


Quote :samp:`vendorid` and :samp:`productid`
so YAML keeps leading zeroes and treats the values as strings.
At least one of :samp:`subsystem`, :samp:`vendorid`, or :samp:`productid`
must be set.
If you set :samp:`productid`,
also set :samp:`vendorid`,
because a product ID is only meaningful within a vendor's namespace.


Then reference the in-project SDK from the workshop definition:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 4

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: project-input-sdk


.. note::

   The :samp:`project-` prefix appears only in the :samp:`sdks:` list.
   |ws_markup| strips it internally,
   so the SDK keeps its bare name, :samp:`input-sdk`,
   in connections and command arguments.


Connect the interface
---------------------

Launch the workshop to install the SDK:

.. code-block:: console

   $ workshop launch dev


The custom device interface stays disconnected after launch,
for security reasons.
Connect the plug to the workshop's custom device slot by hand:

.. code-block:: console

   $ workshop connect dev/input-sdk:input-device :custom-device


The first argument is the plug, :samp:`<WORKSHOP>/<SDK>:<PLUG>`.
The trailing :samp:`:custom-device` selects the slot
that the built-in system SDK provides for every workshop.

Connecting the plug makes all existing host devices
that match the plug's declared attributes available inside the workshop.
available inside the workshop.
While the connection is live,
devices attached to the host afterwards appear too,
and detached devices disappear.


Verify the connection
---------------------

List the connections to confirm the plug is wired to the slot.
The :option:`!--all` flag includes disconnected plugs:

.. code-block:: console

   $ workshop connections --all

     INTERFACE      PLUG                        SLOT                      NOTES
     custom-device  dev/input-sdk:input-device  dev/system:custom-device  manual


The :samp:`manual` note marks a connection made by hand
rather than one established automatically at launch.


Access the devices
------------------

Open a shell in the workshop
and list the matching devices:

.. code-block:: console

   $ workshop shell dev
   workshop@dev:~$ ls /dev/input/

     event0  event1  mice


The matching host devices are now reachable inside the workshop.

To revoke access, disconnect the plug:

.. code-block:: console

   $ workshop disconnect dev/input-sdk:input-device


See also
--------

Explanation:

- :ref:`exp_custom_device_interface`
- :ref:`exp_plugs_slots`


How-to guides:

- :ref:`how_declare_plugs_slots`
- :ref:`how_use_multiple_workshops`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_shell`
