.. _exp_plugs_slots:

.. meta::
   :description: Plugs and slots are the mechanism through which SDKs in a
                 workshop expose and consume capabilities, forming the
                 capability topology that connects providers to consumers.

Plugs and slots
===============

.. @artefact plug
.. @artefact slot
.. @artefact interface connection

A workshop is a graph of capabilities.
Each SDK can act as a provider, a consumer, or both,
and the wiring between them
is what lets a workshop deliver a coherent environment
out of independently published parts.

In |ws_markup|, that wiring uses two named endpoints:
*plugs* and *slots*.
Both reference an :ref:`interface <exp_interface_concepts>` type;
slots provide a capability of that type,
and plugs consume one.
The workshop connects matching pairs at launch
and lets you adjust the topology
in the :ref:`workshop definition <exp_workshop_definition_connections>`
when the defaults are not what you want.


Slots provide capabilities
--------------------------

A slot exposes a capability that other SDKs can consume.
What a slot exposes depends on its interface:

- A :ref:`mount interface <exp_mount_interface>` slot
  exposes a directory.

- A :ref:`tunnel interface <exp_tunnel_interface>` slot
  exposes a network endpoint.

- A :ref:`GPU interface <exp_gpu_interface>` slot
  exposes a GPU device.

- A :ref:`camera interface <exp_camera_interface>`,
  :ref:`desktop interface <exp_desktop_interface>`,
  or :ref:`SSH interface <exp_ssh_interface>` slot
  exposes the corresponding host facility.


Some capabilities are inherently host-rooted,
like a camera device or a host-side directory.
Only the :ref:`system SDK <exp_system_sdk>`
can expose host-rooted slots,
which is why every workshop has one installed by default.
Regular SDKs can still publish slots
that expose directories or endpoints from inside the workshop;
a mount slot in a regular SDK,
for example,
points at a path within the SDK or the :samp:`workshop` user's home,
not the host filesystem.


Plugs consume capabilities
--------------------------

A plug is the consumer end.
It is named within the SDK that declares it,
references an interface type,
and carries any attributes the consumer needs to apply
once a slot is connected.
For instance, with a mount plug,
the central attribute is the target path inside the workshop
where the slot's directory should appear.

A plug stays declared even if no slot is connected to it,
which means an SDK can ship optional plugs
that only activate when a corresponding provider is also installed.


.. _exp_interface_autowiring:

Autowiring
----------

When a workshop launches or refreshes,
|ws_markup| tries to connect each plug
to a slot of the same interface type.
The attempt succeeds for each slot the interface policy
permits the plug to connect to.
The policy is what gates auto-connection,
not the number of candidate slots in the workshop.

Auto-connect behavior varies by interface:

- The mount interface auto-connects to slots provided by the system SDK.
  Regular-SDK mount slots are never auto-connected on their own;
  wire them by listing the pair in the workshop's
  :ref:`top-level connections <exp_workshop_definition_connections>`.

- The GPU interface auto-connects.

- The tunnel interface auto-connects only when the slot
  exposes a loopback address.

- The camera, desktop, and SSH interfaces do not auto-connect.
  They have to be wired explicitly.


When more than one slot is policy-eligible for the same plug,
|ws_markup| attempts a connection for each of them
in an order it does not guarantee.
That can land you with multiple connections,
or with a runtime conflict
when the slots collide on a shared resource
(two mount slots targeting overlapping paths, for example).
The right way to express a specific topology
is to write it down in the workshop definition's
:ref:`connections list <exp_workshop_definition_connections>`
instead of leaving it to auto-connection.


Wiring mechanisms
-----------------

The :ref:`workshop definition <exp_workshop_definition>`
gives you two distinct YAML mechanisms for shaping the topology:

- An inline :ref:`plug binding <exp_plug_bindings>`,
  written as :samp:`bind:` inside a plug entry,
  delegates one plug to another plug.
  Both plugs then point at the same target,
  which is how same-interface conflicts are resolved.

- A top-level :samp:`connections:` list,
  written at workshop scope,
  pairs a specific plug with a specific slot.
  Use it whenever the autowiring would be ambiguous
  or when you want to lock down a particular pairing
  even though the defaults happen to land on the right one today.


The two mechanisms are mutually exclusive for a given plug:
a plug that is bound to another plug
cannot also appear in a top-level :samp:`connections:` entry.

|ws_markup| also exposes runtime-only commands,
:command:`workshop connect` and :command:`workshop disconnect`,
that change the wiring of a running workshop.
Those commands do not modify the workshop definition on disk,
so any change they make is lost the next time you rebuild the workshop.
For a persistent decision,
edit the workshop definition.


.. _exp_plug_bindings:

Inline plug bindings
~~~~~~~~~~~~~~~~~~~~

A plug binding lets one plug stand in for another:

.. code-block:: yaml
   :caption: workshop.yaml

   sdks:
     - name: consumer-sdk
       plugs:
         tools:
           bind: provider-sdk:tools


Both plugs then point to the same resource,
and any action performed on one,
such as connecting or remounting,
applies to all bound plugs.

Bindings are the right tool when two plugs of the same interface
would otherwise conflict over the same target,
typically because two SDKs each declare a plug
with overlapping attributes.

A bound plug only carries the binding;
it cannot also define plug attributes of its own.
The attributes come from the plug it is bound to.

When you run :command:`workshop connections`,
a bound plug is shown with :samp:`bind` in the :samp:`NOTES` column,
along with the line number of the plug it points at.


Top-level connections
~~~~~~~~~~~~~~~~~~~~~

The top-level :samp:`connections:` list
pairs a plug with a slot directly:

.. code-block:: yaml
   :caption: workshop.yaml

   connections:
     - plug: consumer-sdk:tools
       slot: provider-sdk:bin


Each entry uses the :samp:`<SDK-NAME>:<NAME>` form
on both sides.
Once the workshop is launched or refreshed,
that pairing is the one |ws_markup| applies,
regardless of what other slots could have matched.

This is the mechanism to use
when the workshop has more than one provider for an interface
and you want to be specific about which one a consumer reads from.


Example: two SDKs sharing a mount
---------------------------------

Consider a workshop that installs two SDKs:

- :samp:`provider-sdk` ships a mount slot named :samp:`bin`
  that exposes a directory inside its own filesystem.

- :samp:`consumer-sdk` declares a mount plug named :samp:`tools`
  whose target is a path under its workshop user's home.


Auto-connection alone does not wire :samp:`consumer-sdk:tools`
to :samp:`provider-sdk:bin`:
the mount interface only auto-connects to system-SDK slots,
so :samp:`consumer-sdk:tools` lands on :samp:`system:mount`
and :samp:`provider-sdk:bin` stays listed but unconnected.
You name the pairing explicitly
with a top-level :samp:`connections:` entry:

.. code-block:: yaml
   :caption: workshop.yaml

   sdks:
     - name: provider-sdk
     - name: consumer-sdk

   connections:
     - plug: consumer-sdk:tools
       slot: provider-sdk:bin


After :command:`workshop launch` or :command:`workshop refresh`,
:command:`workshop connections` shows the chosen pairing
and you can verify that :samp:`consumer-sdk` reads from :samp:`provider-sdk`
rather than the system SDK's default mount slot.


See also
--------

Explanation:

- :ref:`exp_camera_interface`
- :ref:`exp_desktop_interface`
- :ref:`exp_gpu_interface`
- :ref:`exp_interface_concepts`
- :ref:`exp_mount_interface`
- :ref:`exp_sdks`
- :ref:`exp_ssh_interface`
- :ref:`exp_tunnel_interface`
- :ref:`exp_workshop`


How-to guides:

- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_sdk_internals`
- :ref:`ref_sdk_plugs_slots`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_disconnect`
