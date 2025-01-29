.. _exp_workshop_concepts:

Workshop concepts
=================

.. @artefact project
.. @artefact workshop (container)

A *workshop*
(lowercase; not to be confused with |ws_markup| itself)
is a container that is described in a definition file,
which is associated with a :ref:`project directory <exp_projects>`.
Currently, these containers are hosted by `LXD`_,
but it's not recommended to rely on this implementation detail.


.. _exp_workshop_status:

Workshop status
---------------

.. @artefact workshop status

A workshop's life-cycle can see it switch between several statuses:

.. list-table::
   :header-rows: 1

   * - State
     - Description
     - Definition
     - Container

   * - *Off*
     - Initial state; just defined
     - In the project directory
     - Doesn't exist

   * - *Ready*
     - Can perform meaningful work
     - In the project directory
     - Running

   * - *Stopped*
     - Temporarily stopped, can be restarted
     - In the project directory
     - Stopped

   * - *Pending*
     - Being updated, not ready for work
     - In the project directory
     - Running, being updated

   * - *Error*
     - Non-operational
     - Can be missing
     - Can be non-operational


Status diagrams in the `See also`_ section below
provide more details of valid transitions.


.. _exp_workshop_definition:

Workshop definition
-------------------

.. @artefact workshop base image
.. @artefact workshop definition

This is a YAML file
that lists the base image of the workshop
and the specific components installed on top of it.
The definition acts as a single source of truth about the workshop.
It usually takes a few tries
to produce a definition that works for your project,
so you can edit and update the file iteratively.

A simple definition might look like this:

.. code-block:: yaml
   :caption: workshop.yaml

   name: golang
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: latest/stable


.. @artefact SDK
.. @artefact interface

It specifies a *base* and an *SDK*.
A more complete definition would usually list several SDKs
that use different :ref:`interfaces <exp_interfaces>`,
software packages and :ref:`hooks <exp_sdk_hooks>`.

A workshop definition can be hidden by naming it
:file:`.workshop.yaml` instead of :file:`workshop.yaml`.
If a project has multiple workshops,
the definitions should be stored in the :file:`.workshop/` directory
(for example, :file:`.workshop/golang.yaml`).


.. _exp_base:

Base image
~~~~~~~~~~

The base is a supported OS image
that is used as the basis for the workshop.


.. _exp_plug_bindings:

Plug bindings
-------------

When you list an SDK in your workshop,
you can optionally *bind* any of its plugs
to other plugs of the same :ref:`interface <exp_interfaces>`
in the same workshop.

Binding a plug to another plug makes them both refer to a single entity;
any action on a bound plug affects all bindings, and vice versa.
This comes handy if the SDKs implement different features on the same resources
or simply use a singleton-like interface (:samp:`gpu` is a good example).

.. @artefact SDK publisher

.. note::

   Double-check the plug layout
   with :command:`workshop connections`;
   you may also need additional info from the SDK publishers.


As an example,
imagine two SDKs, :samp:`pytorch` and :samp:`tensorflow`,
that store their training data in the workshop under
:file:`~/.cache/torchvision/datasets/` and :file:`~/.keras/datasets/`,
respectively.
The data should be persisted,
so each SDK has a corresponding mount interface plug, :samp:`datasets`.

Now, what if our workshop includes both SDKs;
can we leverage bindings to reuse the data?
Here, the :samp:`datasets` plug of the :samp:`pytorch` SDK
is bound to the :samp:`datasets` plug under :samp:`tensorflow`:

.. code-block:: yaml
   :caption: .workshop/digits.yaml
   :emphasize-lines: 8

   name: digits
   base: ubuntu@22.04
   sdks:
     - name: pytorch
       channel: latest/stable
       plugs:
         datasets:
           bind: tensorflow:datasets
     - name: tensorflow
       channel: latest/stable


This binds :samp:`pytorch:datasets`
to the location of :samp:`tensorflow:datasets`;
you benefit from sharing the data between the two frameworks,
while simultaneously persisting it on the host.

Any actions on the plug thus bound affect all its bindings.
Here, if you remount :samp:`pytorch:datasets`,
the :samp:`tensorflow:datasets` plug is also remounted
because they reference the same entity:

.. @artefact workshop info

.. code-block:: console

   $ workshop remount digits/pytorch:datasets /new-mount/
   $ workshop info digits

     ...
     sdks:
       pytorch:
         tracking:   latest/stable
         installed:  2.5.1  2024-11-02  (42)
         mounts:
           datasets:
             host-source:      /new-mount
             workshop-target:  /home/workshop/.cache/torchvision/datasets
       tensorflow:
         tracking:   latest/stable
         installed:  2.18.0  2024-10-27  (37)
         mounts:
           data:
             host-source:      /new-mount
             workshop-target:  /home/workshop/.keras/datasets


This avoids the need to reconfigure each mount manually,
reducing the potential for mistakes.

When you run :command:`workshop connections`,
a bound plug will have :samp:`bind` listed under :samp:`Notes`,
along with the line number of the target plug:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections digits

     Interface  Plug                        Slot                 Notes
     mount      digits/pytorch:datasets     digits/system:mount  bind.2
     mount      digits/tensorflow:datasets  digits/system:mount  bind.2


Here, both plugs are listed as :samp:`bind.2`,
pointing to :samp:`tensorflow:datasets` in the second line.


.. _exp_workshop_definition_connections:

Slots, plugs, connections
-------------------------

You can declare :ref:`slots or plugs <exp_plugs_slots>`
and list connections in the workshop definition,
subject to the usual :ref:`validation rules <exp_interfaces_validation>`.
This reduces the need to run manual commands after starting the workshop.

This example adds a slot, a plug and two connections to its SDKs:

.. code-block:: yaml
   :caption: .workshop/digits-cuda.yaml
   :emphasize-lines: 5-8, 11-14, 17-21

   base: ubuntu@22.04
   name: digits-cuda
   sdks:
     - name: system
       slots:
         images:
           interface: mount
           workshop-source: /project/training-data/low-res
     - name: tensorflow
       channel: latest/stable
       plugs:
         cuda:
           interface: mount
           workshop-target: /usr/local/cuda/lib64
     - name: cuda
       channel: latest/stable
   connections:
     - plug: tensorflow:cuda
       slot: cuda:libs
     - plug: tensorflow:images
       slot: system:images


Here, :samp:`system:images`
is a :ref:`mount interface <exp_mount_interface>` slot,
whose :samp:`workshop-source` attribute points to a directory in the workshop.
At run-time, the :samp:`tensorflow:images` plug is connected to the slot
to consume the data from it.

In turn, :samp:`tensorflow:cuda`
is a :ref:`mount interface <exp_mount_interface>` plug
that sets its :samp:`workshop-target` to a directory in the workshop.
At run-time, the plug is connected to the :samp:`cuda:libs` slot,
so the libraries exposed by the slot are available at the plug's target path.

Also, both connections established here
are no different from those created via the command line.


See also
--------

Explanation:

- :ref:`exp_projects`
- :ref:`exp_sdk`


Reference:

- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_status`
