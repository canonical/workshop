.. _exp_sdk:

SDKs
====

SDKs are essential workshop components
that install the required system and language packages,
configure the workshop for their operation
and maintain their own state
throughout the lifetime of the workshop.
An *SDK* is designed by a publisher
and made available via the SDK Store.
A single workshop can include multiple SDKs from different publishers.
SDKs are distributed via
`channels <https://canonical-sdkcraft.readthedocs-hosted.com/en/latest/reference/sdks/#channels>`_
similar to
`snap channels <https://snapcraft.io/docs/channels>`_.


.. _exp_sdk_state:

State
-----

An SDK may store any data specific to it,
such as a model training configuration,
within the workshop.
The publisher of the SDK implements save and restore actions
to let |project_markup| handle such data consistently as the *SDK state*.

Before applying any changes to the workshop
during a :command:`workshop refresh` operation,
|project_markup| saves the workshop's SDK states
by invoking their :ref:`hooks <exp_sdk_hooks>`.
After a successful change,
the states are respectively restored.


.. _exp_sdk_definition:

Definition
----------

An SDK is defined in a file named :file:`sdkcraft.yaml` that may look like this:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   name: go
   title: Go SDK
   base: ubuntu@20.04
   summary: The Go programming language
   description: |
     Go is an open source programming language that enables the production
     of simple, efficient and reliable software at scale.

   plugs:
     mod-cache:
       interface: content
       target: /home/workshop/go/pkg/mod


.. _exp_sdk_hooks:

Life cycle hooks
----------------

SDK publishers can define optional *hooks*
that control and extend the workshop's internal behaviour
to make any framework wrapped as an SDK
compatible with |project_markup|'s logic.

Each hook is a shell script with domain-aware actions
that |project_markup| runs in the workshop
at a certain life cycle phase
to ensure the SDK stays functional.
Specific examples include :samp:`setup-base`,
:samp:`save-state` and :samp:`restore-state`.

You may see individual hooks mentioned
when running :program:`workshop changes` and :program:`workshop tasks` commands;
understanding the events that trigger them may help you with troubleshooting.


.. _exp_interfaces:

Interfaces
----------

To make SDKs customisable and extensible,
|project_markup| implements a counterpart to
:program:`snapd`'s
`interface manager <https://snapcraft.io/docs/interface-management>`__,
controlling whether an individual SDK can use resources beyond its confinement.
You can think of specific interfaces as resource *types*:
file system, hardware, computational and so on.


.. _exp_interfaces_plugs_slots:

Plugs and slots
~~~~~~~~~~~~~~~

In order to provide access to these resource types,
|project_markup| exposes so-called *interface slots*.
For instance, a :ref:`content interface slot <exp_content_interface>`
creates a designated host directory to be mounted inside the workshop;
think of the slot as the provider of the resource.

On top of that, individual SDKs define *plugs*
to connect to a slot that belongs to a certain interface.
In our :ref:`previous example <exp_sdk_definition>`,
it's the aforementioned *content interface*.

You can think of the plug as the recipient of the resources exposed by the slot;
note that a slot can handle connections with multiple plugs.

Eventually, this mechanism starts whirring when the workshop itself is started;
the plugs defined by its SDKs are automatically connected to the slots,
provided the definition contains everything |project_markup| needs to make a match.


.. _exp_interfaces_validation:

Validation and policies
~~~~~~~~~~~~~~~~~~~~~~~

Now, to make sure plugs can be installed and auto-connected,
|project_markup| uses a set of rules called policies,
with each interface having its own.
For example, the content interface plug can be installed and auto-connected
based on its policy alone.
However, other interfaces may have different rules,
such as enabling installation but not auto-connection for :samp:`ssh-agent`.

Finally, when all checks are done,
the SDKs are able to use the external resources.


.. _exp_interfaces_cli_operations:

Related CLI operations
~~~~~~~~~~~~~~~~~~~~~~

A number of basic workshop operations
affect plugs and slots in different ways.

When you :command:`launch` a workshop,
an auto-connect task handles the content interface plug,
finding a candidate slot,
verifying the plug's eligibility for the slot based on their declarations
and connecting the two.

Upon :command:`refresh`,
existing connections are preserved in the refreshed workshop
if their plugs were connected before the operation.
A newer version of an SDK may drop a plug that was previously connected;
such connections are removed,
but the content remains.

On :command:`remove`,
both the interface connections and the host directories
(if any were created, for example, to accommodate content slots)
are removed.

.. note::

   We remove the content from the default locations
   because it's not a good idea to keep user data forever.
   Thus, at least some workshop operations will delete this data
   to prevent it from piling up in hidden locations,
   where it's unlikely to be used again.


See also
--------

Explanation:

- :ref:`project (concept) <exp_project>`
- :ref:`workshop (concept) <exp_workshop>`


Reference:

- :ref:`ref_workshop_cli`
- :ref:`ref_sdk_hooks`