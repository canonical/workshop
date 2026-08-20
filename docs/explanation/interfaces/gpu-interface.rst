.. _exp_gpu_interface:

.. meta::
   :description: Documentation of the GPU interface that enables workshops to
                 access the host system's GPUs directly, allowing for improved
                 performance of GPU-intensive applications and workloads in
                 isolated environments.

GPU interface
=============

.. @artefact GPU interface

The GPU interface
enables GPU pass-through
(direct access to the host system's GPUs)
inside the workshop
to improve the performance of GPU-intensive applications.

By using the interface,
the SDK publisher allows the workshop to directly access the host's GPU devices,
which may be required for various GPU-intensive workloads.

.. _exp_gpu_plug:

GPU interface plug
------------------

An essential element here is the GPU interface plug,
which is declared in the SDK definition.

Its structure includes just the name of the plug and the interface;
both must be set to :samp:`gpu`.

Defining the plug in an SDK
allows the workshops using this SDK to directly access the host's GPU devices,
which may be required for various GPU-intensive workloads.


.. _exp_gpu_slot:

GPU interface slot
------------------

To let SDKs in a workshop access the host's GPUs,
|ws_markup| provides a GPU interface slot
that multiple GPU interface plugs can access.

When the SDK is installed at runtime during launch and refresh operations,
|ws_markup| checks that the plug targeting the slot
passes :ref:`validation <exp_interfaces_validation>`;
if it does,
it can be connected.


Connection
----------

The interface is connected automatically at launch or refresh,
provided that the plug can be matched to the slot by its name
or via a :samp:`connections` entry in the :ref:`definition <exp_workshop_definition>`,
both subject to |ws_markup|'s
:ref:`validation rules <exp_interfaces_validation>`.

After the workshop has started,
the :command:`workshop connect` and :command:`workshop disconnect` commands
can be used to manage the connection manually.

Establishing a connection means
the host's GPUs are directly available inside the workshop
via the GPU pass-through mechanism.

To check if the interface is connected:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections --all

     INTERFACE  PLUG            SLOT           NOTES
     ...
     gpu        ws/gpu-sdk:gpu  ws/system:gpu  -


This means the host's GPUs are directly available inside the workshop:

.. @artefact workshop shell

.. code-block:: console

   $ workshop shell ws
   workshop@ws:/project$ ls -h /dev/dri/

     card0  renderD128

   workshop@ws:/project$ nvidia-smi


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
