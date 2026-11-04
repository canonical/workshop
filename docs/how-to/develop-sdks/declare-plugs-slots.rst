.. _how_declare_plugs_slots:

.. meta::
   :description: Step-by-step guide on declaring mount and tunnel plugs and
                 slots in an SDK definition so that an SDK can consume and
                 expose capabilities to other SDKs in a workshop.

How to declare plugs and slots
==============================

.. @tests in tests/docs-how-to/declare-plugs-slots/task.yaml

.. @artefact interface plug
.. @artefact interface slot
.. @artefact sdkcraft (CLI)

This guide shows how to declare plugs and slots
in an SDK definition,
so that an SDK can consume capabilities from other SDKs
or expose its own to them.
The examples cover the :samp:`mount` and :samp:`tunnel` interfaces;
these are the interfaces for which a regular SDK
can declare both plugs and slots.
Other interfaces have their own ownership rules.
For example,
a regular SDK can declare a :samp:`secret` plug,
but only the :samp:`system` SDK can provide its slot.
To receive a credential through that plug,
see :ref:`how_use_secrets`.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- |sdk_markup| installed.

- An :file:`sdkcraft.yaml` you can edit.
  If you don't have one yet,
  :ref:`tut_craft_sdks` walks through scaffolding an SDK with
  :command:`sdkcraft init`.

The declarations below go under top-level
:samp:`plugs:` and :samp:`slots:` keys in :file:`sdkcraft.yaml`.


Declare a mount plug
--------------------

A mount plug consumes a directory
that becomes available at a path inside the workshop.
The required attribute is :samp:`workshop-target`,
which must be an absolute path
and may use :envvar:`$SDK` to refer to the SDK installation directory:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-6

   # ...

   plugs:
     cache:
       interface: mount
       workshop-target: /home/workshop/.cache/cachekit


When a workshop installs the SDK,
|ws_markup| auto-connects this plug to :samp:`system:mount`,
the slot the system SDK provides,
and the plug receives a directory allocated on the host.
Pairing it with another SDK's slot instead
requires an explicit :samp:`connections:` entry in the workshop definition;
:ref:`how_share_content_between_sdks` covers that flow end to end.
The :samp:`mode`, :samp:`uid`, :samp:`gid`,
and :samp:`read-only` attributes are optional.


Declare a mount slot
--------------------

A mount slot exposes a directory the SDK provides
so that other SDKs can plug into it.
The required attribute is :samp:`workshop-source`,
which must be an absolute path inside the workshop
and may use :envvar:`$SDK`:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-6

   # ...

   slots:
     shared:
       interface: mount
       workshop-source: /home/workshop/cachekit-share


This is for cross-SDK sharing within the workshop.
A mount slot on a regular SDK is not wired on its own:
until a workshop definition names it
in a :samp:`connections:` entry,
it stays declared and unconsumed.
:ref:`how_share_content_between_sdks` covers that flow end to end.

Exposing a directory from the host
is the responsibility of the system SDK;
a regular SDK cannot declare a host-rooted mount slot.


Declare a tunnel slot
---------------------

A tunnel slot exposes a network endpoint
inside the workshop:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 3-6

   # ...

   slots:
     api:
       interface: tunnel
       endpoint: 127.0.0.1:8080


A tunnel slot is auto-connected only when the host side
declares a tunnel plug that matches the slot by name
or through a :samp:`connections:` entry in the workshop definition,
and only when that plug's endpoint
is a loopback address or a Unix domain socket.
Other pairings have to be connected manually
with :command:`workshop connect`.
The endpoint syntax accepts shorthand forms,
including bare port numbers and unix socket paths.
See :ref:`ref_tunnel_interface` for the full grammar.


See also
--------

Explanation:

- :ref:`exp_mount_interface`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdks`
- :ref:`exp_system_sdk`
- :ref:`exp_tunnel_interface`
- :ref:`exp_workshop_definition_connections`


How-to guides:

- :ref:`how_build_sdk`
- :ref:`how_configure_mount`
- :ref:`how_resolve_plug_conflicts`
- :ref:`how_share_content_between_sdks`


Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_plugs_slots`
- :ref:`ref_tunnel_interface`


Tutorial:

- :ref:`tut_craft_sdks`
