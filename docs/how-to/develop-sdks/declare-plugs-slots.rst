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
plugs and slots for the other supported interfaces
follow the same shape.


Prerequisites
-------------

You need a working |sdk_markup| installation
and an :file:`sdkcraft.yaml` you can edit.
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
The :samp:`mode`, :samp:`uid`, and :samp:`gid` attributes are optional.


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


Auto-connection for the tunnel interface is gated by the *plug* endpoint:
|ws_markup| auto-connects a tunnel plug only when its endpoint
is a loopback address or a Unix domain socket.
Plugs that listen on other addresses
require an explicit
:ref:`connection in the workshop definition <exp_workshop_definition_connections>`.
The endpoint syntax accepts shorthand forms,
including bare port numbers and unix socket paths.
See :ref:`ref_tunnel_interface` for the full grammar.


Verify the declarations
-----------------------

After packing the SDK with :command:`sdkcraft pack`,
confirm that the declared plugs and slots
ended up in the SDK metadata by extracting :file:`meta/sdk.yaml`
from the artifact:

.. code-block:: console

   $ tar xOf <NAME>_<ARCH>_<BASE>.sdk meta/sdk.yaml


The output should include the :samp:`plugs:` and :samp:`slots:` blocks
exactly as they were declared.


See also
--------

Explanation:

- :ref:`exp_mount_interface`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdks`
- :ref:`exp_tunnel_interface`


How-to guides:

- :ref:`how_design_interface_layout`
- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_plugs_slots`
- :ref:`ref_tunnel_interface`


Tutorial:

- :ref:`tut_craft_sdks`
