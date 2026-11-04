.. _exp_plugs_slots:

.. meta::
   :description: Plugs and slots are the mechanism through which SDKs in a
                 workshop expose and consume capabilities, forming the
                 capability topology that connects providers to consumers.

Plugs and slots
===============

.. @artefact interface plug
.. @artefact interface slot
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

A slot exposes a capability that other SDKs can consume;
a single slot can serve connections from multiple plugs at once.
What a slot exposes depends on its interface:

- A :ref:`mount interface <exp_mount_interface>` slot
  exposes a directory.

- A :ref:`tunnel interface <exp_tunnel_interface>` slot
  exposes a network endpoint.

- A :ref:`GPU interface <exp_gpu_interface>` slot
  exposes a GPU device.

- A :ref:`secret interface <exp_secret_interface>` slot
  exposes a credential from the host keyring.

- A :ref:`camera interface <exp_camera_interface>`,
  :ref:`custom device interface <exp_custom_device_interface>`,
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


.. _exp_interface_auto_connection:

Auto-connection
---------------

When a workshop launches or refreshes,
|ws_markup| tries to connect each plug
to a slot of the same interface type.
The attempt succeeds when the interface policy
permits the plug to connect to a candidate slot.
The policy is what gates auto-connection,
not the number of candidate slots in the workshop.

Auto-connection behavior, and which SDK type may declare each endpoint,
varies by interface.
In the SDK-type columns, *any* means either a regular SDK or the system SDK.

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 24 22 22 16

   * - Interface
     - Plug SDK type
     - Slot SDK type
     - Auto-connection

   * - :ref:`gpu <exp_gpu_interface>`
     - regular
     - system
     - Yes

   * - :ref:`mount <exp_mount_interface>`
     - regular
     - any
     - Partial :sup:`*`

   * - :ref:`tunnel <exp_tunnel_interface>`
     - any
     - any
     - Partial :sup:`**`

   * - :ref:`camera <exp_camera_interface>`
     - regular
     - system
     - No

   * - :ref:`custom-device <exp_custom_device_interface>`
     - regular
     - system
     - No

   * - :ref:`desktop <exp_desktop_interface>`
     - regular
     - system
     - No

   * - :ref:`secret <exp_secret_interface>`
     - regular
     - system
     - No

   * - :ref:`ssh-agent <exp_ssh_interface>`
     - regular
     - system
     - No

:sup:`*` Left to the SDKs' own rules,
a mount plug auto-connects only to the slot
that the :ref:`system SDK <exp_system_sdk>` provides.
Naming a regular SDK's slot in a :samp:`connections:` entry
makes that pair eligible too.

:sup:`**` Tunnel auto-connects only from host to workshop,
between a plug and a slot of the same name,
and only when the plug's endpoint
is a loopback address or a Unix domain socket.
See :ref:`exp_tunnel_connection` for the full policy.

Interfaces marked No are wired manually
with :command:`workshop connect`.


When more than one slot is policy-eligible for the same plug,
|ws_markup| attempts a connection for each of them
in an order it does not guarantee.
The right way to express a specific topology
is to write it down in the workshop definition's :samp:`connections:` list
instead of leaving the target to the interface's default.


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
  Use it to override the slot a plug would otherwise auto-connect to,
  for example to wire a mount plug to a regular SDK's slot
  rather than the system SDK's.
  The pair still has to satisfy the interface's auto-connection policy,
  so interfaces that block auto-connection outright
  (such as :samp:`secret` and :samp:`ssh-agent`)
  cannot be wired this way
  and must be connected with :command:`workshop connect`.


The two mechanisms are mutually exclusive for a given plug:
if it's bound to another plug,
it cannot also appear in a top-level :samp:`connections:` entry.

|ws_markup| also exposes the runtime-only
:command:`workshop connect` and :command:`workshop disconnect` commands,
which change the wiring of a running workshop.


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
both the bound plug and its target
carry a :samp:`bind.<N>` note in the :samp:`NOTES` column,
where :samp:`<N>` is the row number of the target plug
in the same output.


Top-level connections
~~~~~~~~~~~~~~~~~~~~~

The top-level :samp:`connections:` list
pairs a plug with a slot directly:

.. code-block:: yaml
   :caption: workshop.yaml

   connections:
     - plug: jupyter:venv
       slot: uv:venv


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

- :samp:`uv` ships a mount slot named :samp:`venv`
  that exposes the Python virtual environment it builds.

- :samp:`jupyter` declares a mount plug named :samp:`venv`
  whose target is a path inside its own SDK directory.


Without configuration, |ws_markup| does not wire :samp:`jupyter:venv`
to :samp:`uv:venv`:
the mount interface auto-connects to system SDK slots by default,
so :samp:`jupyter:venv` lands on :samp:`system:mount`
and :samp:`uv:venv` stays listed but unconnected.
Matching names count for nothing here;
you name the pairing explicitly
with a top-level :samp:`connections:` entry:

.. code-block:: yaml
   :caption: workshop.yaml

   sdks:
     - name: uv
     - name: jupyter

   connections:
     - plug: jupyter:venv
       slot: uv:venv


After :command:`workshop launch` or :command:`workshop refresh`,
:command:`workshop connections` shows the chosen pairing
and you can verify that :samp:`jupyter` reads from :samp:`uv`
rather than the system SDK's default mount slot.


See also
--------

Explanation:

- :ref:`exp_camera_interface`
- :ref:`exp_custom_device_interface`
- :ref:`exp_desktop_interface`
- :ref:`exp_gpu_interface`
- :ref:`exp_interface_concepts`
- :ref:`exp_mount_interface`
- :ref:`exp_sdks`
- :ref:`exp_secret_interface`
- :ref:`exp_ssh_interface`
- :ref:`exp_tunnel_interface`
- :ref:`exp_workshop`


How-to guides:

- :ref:`how_add_mounts`
- :ref:`how_declare_plugs_slots`
- :ref:`how_resolve_plug_conflicts`
- :ref:`how_share_content_between_sdks`


Reference:

- :ref:`ref_sdk_internals`
- :ref:`ref_sdk_plugs_slots`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_disconnect`
