.. _exp_interface_concepts:

Interface concepts
==================

.. @artefact SDK
.. @artefact interface

In |ws_markup|, SDKs can act as providers and consumers of resources
such as the GPU or file directories.
Host system resources
are exposed via the :ref:`system SDK <exp_system_sdk>`
that is present in every workshop by design.

For a workshop to be operational, the SDKs listed in its definition
must *connect* to the resources they expect.
Such connections are uniformly established via the interface system.


.. _exp_interface_connections:

Connections
-----------

.. @artefact interface connection

Interface connections are a mechanism for communication and resource sharing.
It is an integral part of workshop confinement,
ensuring that each workshop operates in its own isolated environment,
while still allowing controlled interactions among the SDKs and with the system.

Here's how it works from the outside:

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
This behaviour varies by interface,
but the overall aim is to conduct reasonably in each case:
the :ref:`mount <exp_mount_interface>`
and the :ref:`GPU <exp_gpu_interface>` interfaces are auto-connected,
whereas the :ref:`camera <exp_camera_interface>`,
:ref:`desktop <exp_desktop_interface>` and :ref:`SSH <exp_ssh_interface>`
interfaces require manual connection.

See also
--------

Explanation:

- :ref:`exp_workshop`
- :ref:`exp_sdk`


Reference:

- :ref:`ref_cli`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_disconnect`