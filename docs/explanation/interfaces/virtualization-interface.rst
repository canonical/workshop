.. _exp_virtualization_interface:

.. meta::
   :description: Documentation of the virtualization interface that enables
                 workshops to run hardware-accelerated virtual machines by
                 passing through the host's KVM and vhost/vsock character
                 devices.

Virtualization interface
========================

.. @artefact virtualization interface

The virtualization interface
enables hardware-accelerated virtual machines inside a workshop
by passing through the host's virtualization character devices.

By using the interface,
the SDK publisher allows the workshop to directly access the host's
:file:`/dev/kvm` device (for KVM acceleration)
together with the :samp:`vhost` and :samp:`vsock` devices
that accelerate virtual machine networking and host/guest communication.


.. _exp_virtualization_plug:

Virtualization interface plug
-----------------------------

An essential element here is the virtualization interface plug,
which is declared in the SDK definition.

Its structure includes just the name of the plug and the interface;
both must be set to :samp:`virtualization`.

Defining the plug in an SDK
allows the workshops using this SDK to run hardware-accelerated
virtual machines.


.. _exp_virtualization_slot:

Virtualization interface slot
-----------------------------

To let SDKs in a workshop access the host's virtualization devices,
|ws_markup| provides a virtualization interface slot
that multiple virtualization interface plugs can access.

When the SDK is installed at runtime during launch and refresh operations,
|ws_markup| checks that the plug targeting the slot
passes :ref:`validation <exp_interfaces_validation>`;
if it does,
it can be connected.


Devices and permissions
-----------------------

When the interface is connected,
the following host devices are exposed inside the workshop
as :samp:`unix-char` devices:

- :file:`/dev/kvm`
- :file:`/dev/vhost-net`
- :file:`/dev/vhost-vsock`
- :file:`/dev/vsock`

The devices are owned by the workshop user with mode :samp:`0660`,
so the passed-through devices are accessible without granting world access.
Devices that are not present on the host
(for example when the :samp:`vsock` kernel modules are not loaded)
are skipped and do not prevent the workshop from starting.

:file:`/dev/net/tun`, which virtual machine networking also relies on,
is already available inside the workshop
and therefore is not part of this list.


Connection
----------

Unlike the GPU interface,
the virtualization interface is not connected automatically;
it must be connected manually
because it grants a workshop privileged access to host virtualization devices.

The plug can be matched to the slot by its name
or via a :samp:`connections` entry in the :ref:`definition <exp_workshop_definition>`,
both subject to |ws_markup|'s
:ref:`validation rules <exp_interfaces_validation>`.

After the workshop has started,
the :command:`workshop connect` and :command:`workshop disconnect` commands
can be used to manage the connection manually.

Establishing a connection means
the host's virtualization devices are directly available inside the workshop.

To check if the interface is connected:

.. code-block:: console

   $ workshop connections --all

     INTERFACE       PLUG                          SLOT                      NOTES
     ...
     virtualization  ws/vm-sdk:virtualization      ws/system:virtualization  -


This means the host's virtualization devices are directly available
inside the workshop:

.. code-block:: console

   $ workshop shell ws
   workshop@ws-8584e571$ ls -h /dev/kvm /dev/vhost-net /dev/vsock

     /dev/kvm  /dev/vhost-net  /dev/vsock


See also
--------

Explanation:

- :ref:`exp_interface_concepts`
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
