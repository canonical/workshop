.. _how_declare_plugs_slots:

.. meta::
   :description: Step-by-step guide on declaring mount and tunnel plugs and
                 slots in an SDK definition so that an SDK can consume and
                 expose capabilities to other SDKs in a workshop.

How to declare plugs and slots
==============================

.. @artefact interface plug
.. @artefact interface slot
.. @artefact sdkcraft (CLI)

This guide shows how to declare plugs and slots
in an SDK definition,
so that an SDK can consume capabilities from other SDKs
or expose its own to them.
The examples cover the :samp:`mount` and :samp:`tunnel` interfaces;
plugs and slots for the other supported interfaces
follow the same shape.


Prerequisites
-------------

You need a working |sdk_markup| installation
and an editor for :file:`sdkcraft.yaml`.
:ref:`tut_craft_sdks` walks through the surrounding workflow
and is a useful reference for the commands used below.


Scaffold an SDK
---------------

Create an empty directory named after the SDK
and run :command:`sdkcraft init`:

.. code-block:: console

   $ mkdir cachekit/ && cd cachekit/
   $ sdkcraft init


This produces a starting :file:`sdkcraft.yaml`
and a :file:`hooks/` directory with sample :samp:`setup-base`
and :samp:`setup-project` scripts.
The default :file:`sdkcraft.yaml` does not declare any plugs or slots;
add them under top-level :samp:`plugs:` and :samp:`slots:` keys.


Declare a mount plug
--------------------

A mount plug consumes a directory
that becomes available at a path inside the workshop.
The required attribute is :samp:`workshop-target`,
which must be an absolute path
and may use :envvar:`$SDK` to refer to the SDK installation directory:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-5

   # ...

   plugs:
     cache:
       interface: mount
       workshop-target: /home/workshop/.cache/cachekit


When a workshop installs the SDK,
|ws_markup| connects this plug
to a matching slot,
either auto-connecting it to the workshop's :ref:`system SDK <exp_system_sdk>`
or to another SDK's slot
when the workshop definition wires that pairing explicitly.

.. note::

   Defaults for :samp:`uid`, :samp:`gid`, and :samp:`mode`
   depend on the target path.
   If the plug needs explicit ownership or permissions,
   see :ref:`how_configure_mount_ownership`.


Declare a mount slot
--------------------

A mount slot exposes a directory the SDK provides
so that other SDKs can plug into it.
The required attribute is :samp:`workshop-source`,
which must be an absolute path inside the workshop
and may use :envvar:`$SDK`:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-5

   # ...

   slots:
     shared:
       interface: mount
       workshop-source: /home/workshop/cachekit-share


This is for cross-SDK sharing within the workshop.
Exposing a directory from the host
is the responsibility of the
:ref:`system SDK <exp_system_sdk>`;
a regular SDK cannot declare a host-rooted mount slot.


Declare a tunnel slot
---------------------

A tunnel slot exposes a network endpoint
inside the workshop:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-5

   # ...

   slots:
     api:
       interface: tunnel
       endpoint: 127.0.0.1:8080


|ws_markup| auto-connects tunnel pairings only when the endpoint
is on a loopback address;
endpoints bound to other addresses
require an explicit
:ref:`connection in the workshop definition <exp_workshop_definition_connections>`.
The endpoint syntax accepts shorthand forms,
including bare port numbers and unix socket paths.
See :ref:`ref_tunnel_interface` for the full grammar.


Pack and inspect
----------------

Run :command:`sdkcraft pack` to build the SDK:

.. code-block:: console

   $ sdkcraft pack


|sdk_markup| produces one :file:`<NAME>_<ARCH>_<BASE>.sdk`
artifact per platform listed in :file:`sdkcraft.yaml`.
The artifact is a zstd-compressed tarball
that embeds the SDK metadata under :file:`meta/sdk.yaml`.
Confirm that the declared plugs and slots
ended up in the metadata by extracting it:

.. code-block:: console

   $ tar xOf cachekit_amd64_ubuntu@22.04.sdk meta/sdk.yaml


The output should include the :samp:`plugs:` and :samp:`slots:` blocks
exactly as they were declared,
plus the :samp:`base:` and :samp:`architecture:` fields
that |sdk_markup| derives from the platform matrix.

The packed SDK is now ready to be tried locally with
:command:`sdkcraft try`
or uploaded to the SDK Store.


See also
--------

Explanation:

- :ref:`exp_mount_interface`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdks`
- :ref:`exp_tunnel_interface`


How-to guides:

- :ref:`how_configure_mount_ownership`
- :ref:`how_design_interface_layout`
- :ref:`how_connect_plug_slot_pairs_explicitly`
- :ref:`how_resolve_plug_conflicts`
- :ref:`how_write_runtime_hooks`


Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_plugs_slots`
- :ref:`ref_tunnel_interface`


Tutorial:

- :ref:`tut_craft_sdks`
