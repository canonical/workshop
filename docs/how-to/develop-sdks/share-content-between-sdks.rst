.. _how_share_content_between_sdks:

.. meta::
   :description: How-to guide on building a shared directory into two SDKs you
                 author, covering the mount slot on the providing side, the
                 mount plug on the consuming side, and the connections entry
                 that pairs them in a workshop.

How to share content between SDKs
=================================

.. @tests in tests/docs-how-to/share-content-between-sdks/task.yaml

Two SDKs in the same workshop can share a directory
through the mount interface,
without routing the content through the host.
Three pieces make up the flow:
the providing SDK declares a mount slot,
the consuming SDK declares a mount plug,
and the workshop definition pairs the two
with an explicit :samp:`connections:` entry.

Where the SDKs already declare the slot and the plug,
only the pairing is left to graft on
from the workshop definition,
and :ref:`how_add_mounts` covers that
with the shipped :samp:`uv` and :samp:`jupyter` pair.
Building the slot and the plug into SDKs you author
is the subject here.
The examples use two synthesized SDKs:
:samp:`cachekit`, which publishes a directory,
and :samp:`builder-sdk`, which reads it.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- A working |sdk_markup| installation.

- An :file:`sdkcraft.yaml` you can edit
  for each side of the pair.
  If you don't have one yet,
  :ref:`tut_craft_sdks` walks through scaffolding an SDK with
  :command:`sdkcraft init`.

- A workshop you can launch and refresh
  on a host with |ws_markup| installed.


Declare the mount slot
----------------------

The providing SDK exposes a directory with a mount slot.
:samp:`cachekit` publishes the directory it fills
so that other SDKs can read it:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-6

   # ...

   slots:
     shared:
       interface: mount
       workshop-source: /home/workshop/cachekit-share


:samp:`workshop-source` is the only attribute a mount slot accepts,
and :ref:`how_declare_plugs_slots` covers its form.
In particular, a slot cannot mark itself read-only:
:samp:`read-only` is a plug attribute,
so each consumer decides for itself
whether it mounts the shared directory read-only.

The slot publishes the directory but never creates it.
Make sure the SDK does,
in a :samp:`setup-base` or :samp:`setup-project` hook,
or through the parts that build it.


Declare the mount plug
----------------------

The consuming SDK declares a mount plug
naming the path where the shared directory appears.
:samp:`builder-sdk` reads what :samp:`cachekit` publishes
through a plug of its own:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-6

   # ...

   plugs:
     cache:
       interface: mount
       workshop-target: /home/workshop/builder-sdk-cache


Neither declaration names the other SDK,
and the plug and the slot don't have to share a name:
:samp:`builder-sdk` calls its plug :samp:`cache`
while :samp:`cachekit` calls its slot :samp:`shared`.
Nothing pairs them until the workshop definition does.


Connect the SDKs
----------------

Listing both SDKs in a workshop is not enough to pair them.
Left to the SDKs' own rules,
a mount plug auto-connects only to the slot the :samp:`system` SDK provides,
so :samp:`builder-sdk:cache` connects to :samp:`system:mount`
and receives a directory that |ws_markup| allocates on the host,
while :samp:`cachekit:shared` stays listed but unconsumed.

To pair them, name the plug and the slot
in a top-level :samp:`connections:` entry:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 6-8

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: cachekit
     - name: builder-sdk
   connections:
     - plug: builder-sdk:cache
       slot: cachekit:shared


Both sides use the :samp:`<SDK-NAME>:<NAME>` form.
While the pair is still unpublished,
list the SDKs under :samp:`sdks:`
with the :samp:`try-` or :samp:`project-` prefix
that matches how you're testing them.
The prefix belongs in :samp:`sdks:` only:
:samp:`connections:` always names the bare SDK,
and a prefixed name there fails the launch as a reserved name.

Apply the change with :command:`workshop launch`,
or :command:`workshop refresh` for a workshop that is already running:

.. code-block:: console

   $ workshop refresh


.. note::

   A plug named in :samp:`connections:` is claimed by that entry.
   It no longer falls back to :samp:`system:mount`,
   so the host directory it used to receive is no longer mounted.
   Removing the entry and refreshing again returns the plug to that default.


Verify the connection
---------------------

Confirm the pairing with :command:`workshop connections`:

.. code-block:: console

   $ workshop connections dev

     INTERFACE  PLUG                   SLOT                 NOTES
     mount      dev/builder-sdk:cache  dev/cachekit:shared  -
     mount      dev/builder-sdk:state  dev/system:mount     -


On the first row,
the :samp:`SLOT` column names :samp:`dev/cachekit:shared`
rather than :samp:`dev/system:mount`,
which confirms that :samp:`builder-sdk` reads from :samp:`cachekit`
instead of from a host directory.

The second row shows the default the :samp:`connections:` entry overrides.
:samp:`builder-sdk:state` is not named in the workshop definition,
so it auto-connects to :samp:`system:mount`
and receives a host directory.
A dash in the :samp:`PLUG` column marks a slot that nothing consumes,
which is what :samp:`dev/cachekit:shared` would show
if the :samp:`connections:` entry were missing.


See also
--------

Explanation:

- :ref:`exp_best_dependencies`
- :ref:`exp_mount_interface`
- :ref:`exp_plugs_slots`
- :ref:`exp_system_sdk`
- :ref:`exp_workshop_definition_connections`


How-to guides:

- :ref:`how_add_mounts`
- :ref:`how_declare_plugs_slots`
- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`


Tutorial:

- :ref:`tut_craft_sdks`
