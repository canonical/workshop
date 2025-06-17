.. _exp_sdk_concepts:

SDK concepts
============

.. @artefact SDK
.. @artefact SDK publisher
.. @artefact SDK Store

An *SDK* is an essential workshop component
that installs system and language packages,
configures the workshop for their operation
and maintains its own state
throughout the lifetime of the workshop.
Most SDKs are designed by publishers
and made available via the SDK Store,
but some are specific to a particular project or user.
A single workshop can include multiple SDKs from different sources.
SDKs are distributed through channels similar to
`snap channels <https://snapcraft.io/docs/channels>`_.


.. _exp_sdk_definition:

SDK definition
--------------

.. @artefact SDK definition

An SDK is defined by the SDK publisher;
the definition may look like this:

.. @artefact sdkcraft (CLI)

.. code-block:: yaml
   :caption: sdk.yaml

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
that control and extend the workshop's internal behavior
to make any framework wrapped as an SDK
compatible with |ws_markup|'s logic;
in particular, the hooks manage the SDK state
and report its health.

Each hook is a shell script with domain-aware actions
that |ws_markup| runs in the workshop
at a particular life cycle stage
to ensure that the SDK stays functional.
Specific examples include :samp:`setup-base`, :samp:`setup-project`,
:samp:`save-state` and :samp:`restore-state`.

You may see individual hooks mentioned in the output of
:command:`workshop changes` and :command:`workshop tasks`;
understanding the events that trigger them can help you with troubleshooting.


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


.. _exp_system_sdk:

System SDK
----------

.. @artefact system SDK

Every workshop contains a special *system SDK*
that exposes system resources through its slots.
It's unavailable from the SDK Store;
installed first at :command:`workshop launch`
and removed last during :command:`workshop remove`,
it ensures internal consistency.

The purpose of the system SDK isn't to add hooks or additional content;
it's only there to uniformly expose host system resources to other SDKs.
As such, it can't be removed by the user.
It's also the only SDK
that can have :ref:`mount interface <exp_mount_interface>` slots on the host.

The uniformity of this approach lies in the fact that system resources
and workshop resources are exposed using the same logic.
You can also define additional plugs and slots for the system SDK,
just as with any other SDK.

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
