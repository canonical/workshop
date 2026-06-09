.. _exp_interface_concepts:

.. meta::
   :description: A comprehensive explanation of the Workshop interface system,
                 detailing how SDKs connect to host system resources through
                 interfaces, and the mechanism of plugs and slots for resource
                 sharing between containers.

Interface concepts
==================

.. @artefact SDK
.. @artefact interface

Interfaces are a mechanism for communication and resource sharing.
It is an integral part of workshop confinement,
ensuring that each workshop operates in its own isolated environment,
while still allowing controlled interactions among the SDKs and with the host.

In |ws_markup|, SDKs can act as providers and consumers of resources
such as the GPU, or file directories.
Host system resources
are exposed via the :ref:`system SDK <exp_system_sdk>`
that is present in every workshop by design.

For a workshop to be operational, the SDKs listed in its definition
must *connect* to the resources they expect.
Such connections are uniformly established via the interface system.

To achieve this, |ws_markup| implements a counterpart to :program:`snapd`'s
`interface manager <https://snapcraft.io/docs/interface-management/>`__,
which controls whether an SDK can use resources beyond its confines.
You can think of specific interfaces as resource *types*:
filesystem, hardware, computing, and so on.

Specific interfaces are predefined and implemented by |ws_markup|,
so you cannot create a custom interface type.
Currently, |ws_markup| and |sdk_markup| support the following:

- :ref:`Camera interface <exp_camera_interface>` (manually connected)
- :ref:`Custom device interface <exp_custom_device_interface>` (manually connected)
- :ref:`Desktop interface <exp_desktop_interface>` (manually connected)
- :ref:`GPU interface <exp_gpu_interface>` (auto-connected)
- :ref:`Mount interface <exp_mount_interface>` (auto-connected)
- :ref:`SSH interface <exp_ssh_interface>` (manually connected)
- :ref:`Tunnel interface <exp_tunnel_interface>` (conditionally auto-connected)


Plugs and slots
---------------

Interfaces become useful when SDKs declare *plugs* to consume them
and *slots* to provide them.
See :ref:`exp_plugs_slots` for the full mechanics,
including the wiring you can express in the workshop definition.


.. _exp_interfaces_validation:

Validation
----------

All plugs and slots defined for a workshop directly or via its SDKs are checked
to make sure they can be installed as part of the workshop and then connected.
For this, |ws_markup| uses a set of internal rules.

Each interface has its own rule set;
for example, the mount interface plug can be installed and auto-connected
based on its rules alone.
However, other interfaces may have different rules,
such as allowing installation but not auto-connection for :samp:`ssh-agent`.


.. _exp_interface_connections:

Connections
-----------

.. @artefact interface connection

From the user perspective,
connections can be established through the interface system in several ways:

- The :samp:`connections` section of the workshop definition
  and the :command:`workshop connect` command
  can be used to link interface plugs to respective slots,
  allowing the SDKs to orderly access the resources.

- Conversely, the :command:`workshop disconnect` command
  terminates existing interface connections,
  revoking the access to the resources granted by the connection.

- Finally, the :command:`workshop connections` command
  lists all existing connections and their states,
  providing an overview of how workshop connections are laid out.


Some plugs can be auto-connected to their slots at launch or refresh.
This behavior varies by interface,
but the overall aim is to conduct reasonably in each case:
the :ref:`mount <exp_mount_interface>`
and the :ref:`GPU <exp_gpu_interface>` interfaces are auto-connected,
whereas the :ref:`camera <exp_camera_interface>`,
:ref:`custom device <exp_custom_device_interface>`,
:ref:`desktop <exp_desktop_interface>`, and :ref:`SSH <exp_ssh_interface>`
interfaces require manual connection.

Auto-connection also depends on where a plug or slot lives.
Additional slots defined for the system SDK,
for interfaces such as :samp:`tunnel` or :samp:`mount`,
are not auto-connected at launch or refresh,
largely for security reasons:
the system SDK exposes sensitive host system resources.
To the contrary, plugs added under the system SDK can be auto-connected,
because they expose workshop internals.

To know how each kind of connection is treated
when a workshop is launched, refreshed, or restored,
see :ref:`exp_workshop_connection_lifecycle`.


.. _exp_interfaces_cli_operations:

Related CLI operations
----------------------

A number of basic workshop operations
affect plugs and slots in different ways.

.. @artefact workshop launch

When you :command:`workshop launch` a workshop,
an auto-connect task handles each interface plug,
finding a candidate slot,
verifying the plug's eligibility for the slot based on their declarations
and connecting the two.

.. @artefact workshop refresh

On :command:`workshop refresh`,
existing connections are preserved in the refreshed workshop
if their plugs were connected before the operation.
A newer version of an SDK may drop a plug that was previously connected;
such connections are removed,
but the host-based content remains.

.. @artefact interface connection

On :command:`workshop remove`,
both the interface connections and the default host directories
(if any have been created, for example, to accommodate mount interface slots)
are removed.

Also, you can manually enable or disable connections
with :command:`workshop connect` and :command:`workshop disconnect`,
whereas :command:`workshop connections` can list all connections
that have been established by any |ws_markup| projects.

.. note::

   We remove content stored in our default locations
   because it's not a good idea to keep user data forever.
   Thus, at least some commands will delete this data
   to prevent it from piling up in hidden places
   where it's unlikely to be used again.


See also
--------

Explanation:

- :ref:`exp_sdks`
- :ref:`exp_workshop`


How-to guides:

- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_cli`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_info`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_remove`
