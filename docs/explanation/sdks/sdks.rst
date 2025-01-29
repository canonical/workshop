.. _exp_sdk_concepts:

SDK concepts
============

.. @artefact SDK
.. @artefact SDK publisher
.. @artefact SDK Store

SDKs are essential workshop components
that install the required system and language packages,
configure the workshop for their operation
and maintain their own state
throughout the lifetime of the workshop.
An *SDK* is designed by a publisher
and made available via the SDK Store.
A single workshop can include multiple SDKs from different publishers.
SDKs are distributed through channels similar to
`snap channels <https://snapcraft.io/docs/channels>`_.


.. _exp_sdk_state:

SDK state
---------

.. @artefact restore-state
.. @artefact save-state
.. @artefact SDK state

An SDK can store any data specific to it,
such as a model training configuration,
within the workshop.
To enable this,
the SDK publisher implements save and restore :ref:`hooks <exp_sdk_hooks>`
when building the SDK using |sdk_markup|.
Later, |ws_markup| runs these hooks at the appropriate moments
to consistently handle such data, collectively known as *SDK state*.

For example, before changes are applied to the workshop
during :command:`workshop refresh`,
the states of the SDKs are saved
by invoking their :samp:`save-state` hooks.
On success,
they are restored using the :samp:`restore-state` hooks.


.. _exp_sdk_definition:

SDK definition
--------------

.. @artefact SDK definition

An SDK is defined by the SDK publisher;
the definition may look like this:

.. @artefact sdkcraft (CLI)

.. code-block:: yaml
   :caption: sdkcraft.yaml

   name: go
   title: Go SDK
   base: ubuntu@24.04
   summary: The Go programming language
   description: |
     Go is an open source programming language that enables the production
     of simple, efficient and reliable software at scale.

   plugs:
     mod-cache:
       interface: mount
       workshop-target: /home/workshop/go/pkg/mod


.. _exp_hooks:

SDK hooks
---------

.. @artefact SDK hook

SDK publishers can define optional *hooks*
that control and extend the workshop's internal behaviour
to make any framework wrapped as an SDK
compatible with |ws_markup|'s logic;
in particular, the hooks manage the SDK state
and report its health.

Each hook is a shell script with domain-aware actions
that |ws_markup| runs in the workshop
at a particular life cycle stage
to ensure that the SDK stays functional.
Specific examples include :samp:`setup-base`,
:samp:`save-state` and :samp:`restore-state`.

You may see individual hooks mentioned in the output of
:command:`workshop changes` and :command:`workshop tasks`;
understanding the events that trigger them can help you with troubleshooting.


.. _exp_interfaces:

Interfaces
----------

.. @artefact interface

To make SDKs customisable and extensible,
|ws_markup| implements a counterpart to
:program:`snapd`'s
`interface manager <https://snapcraft.io/docs/interface-management>`__,
which controls whether an SDK can use resources beyond its confines.
You can think of specific interfaces as resource *types*:
file system, hardware, computing and so on.

Specific interfaces are predefined and implemented by |ws_markup|,
so you can't create a custom interface type.
Currently, |ws_markup| and |sdk_markup| support the following:

- :ref:`Camera interface <exp_camera_interface>` (manually connected)
- :ref:`Desktop interface <exp_desktop_interface>` (manually connected)
- :ref:`GPU interface <exp_gpu_interface>` (auto-connected)
- :ref:`Mount interface <exp_mount_interface>` (auto-connected)
- :ref:`SSH interface <exp_ssh_interface>` (manually connected)


.. _exp_plugs_slots:

Plugs and slots
~~~~~~~~~~~~~~~

To make use of these interfaces,
SDKs and :ref:`workshops <exp_workshop_definition_connections>` define *slots*.
For example, a :ref:`mount interface <exp_mount_interface>` slot
creates a source directory to be mounted inside the workshop via a plug.

Further, SDKs and :ref:`workshops <exp_workshop_definition_connections>` define *plugs*
to connect to a slot of a certain interface type.
For example, a :ref:`mount interface <exp_mount_interface>` plug
mounts the slot to a target directory inside the workshop.

You can think of the plug as the recipient of the resources exposed by the slot;
note that a slot can handle connections with multiple plugs.

Connections can be established:

- Automatically:
  By running :command:`workshop launch`, :command:`workshop refresh`,
  or :command:`workshop start`.

- Manually:
  By running :command:`workshop connect` after the workshop has started,
  or by listing connections in the
  :ref:`workshop definition <exp_workshop_definition_connections>`
  and running :command:`workshop refresh`.


All connections are subject to validation.
Also, automatic connections require plugs and slots to have matching details
and aren't allowed for some interfaces, such as :samp:`ssh-agent`.


.. _exp_interfaces_validation:

Validation
~~~~~~~~~~

All plugs and slots defined for a workshop directly or via its SDKs are checked
to make sure they can be installed as part of the workshop and then connected.
For this, |ws_markup| uses a set of internal rules.

Each interface has its own rule set;
for example, the mount interface plug can be installed and auto-connected
based on its rules alone.
However, other interfaces may have different rules,
such as allowing installation but not auto-connection for :samp:`ssh-agent`.


.. _exp_interfaces_cli_operations:

Related CLI operations
~~~~~~~~~~~~~~~~~~~~~~

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

.. note::

   We remove content stored in our default locations
   because it's not a good idea to keep user data forever.
   Thus, at least some commands will delete this data
   to prevent it from piling up in hidden places
   where it's unlikely to be used again.


Also, you can manually enable or disable connections
with :command:`workshop connect` and :command:`workshop disconnect`,
whereas :command:`workshop connections` can list all connections
that have been established by any |ws_markup| projects.



.. _exp_system_sdk:

System SDK
~~~~~~~~~~

.. @artefact system SDK

Every workshop contains a special *system SDK*
that exposes system resources through its slots.
It's unavailable from the SDK Store;
installed first at :command:`workshop launch`
and removed last during :command:`workshop remove`,
it ensures internal consistency.

The purpose of the system SDK isn't to add hooks or additional content;
it's only there to uniformly expose host system resources to other SDKs.
As such, it can't be removed by the user
and isn't listed in the :command:`workshop info` output.
It's also the only SDK
that can have :ref:`mount interface <exp_mount_interface>` slots on the host.

The uniformity of this approach lies in the fact that system resources
and workshop resources are exposed using the same logic.
Technically, the system SDK is of :samp:`system` type,
whereas all other SDKs are of :samp:`regular` type,
but this detail isn't exposed in :file:`sdkcraft.yaml`.

.. _exp_sketch_sdk:

Sketch SDK
----------

.. @artefact sketch SDK

The sketch SDK is another special type of SDK.
Again, it's unavailable from the SDK Store;
instead, you define it inside the workshop
using the :command:`workshop sketch-sdk` command.
Its purpose is to allow |ws_markup| users
to quickly make changes to a workshop
beside the regular SDKs listed in the :ref:`definition <exp_sdk_definition>`.

Unlike a regular SDK, the sketch SDK:

- doesn't carry any persistent data
- doesn't appear on the definition
- is unique to the workshop where it was created


The sketch SDK can have :ref:`hooks <exp_sdk_hooks>`
and use :ref:`interfaces <exp_interfaces>`,
which allows it to interact with other SDKs.
Note that :samp:`sketch` is a reserved name,
and the sketch SDK is always installed last.


See also
--------

Explanation:

- :ref:`exp_interface`
- :ref:`exp_projects`
- :ref:`exp_workshop`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_start`
- :ref:`ref_workshop_tasks`
- :ref:`ref_sdk_hooks`
