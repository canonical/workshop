.. _how_bind_plug_slot_pairs_explicitly:

.. meta::
   :description: Resolve ambiguous autowiring in a workshop by binding a plug
                 to a specific slot, either persistently in the workshop
                 definition or transiently with workshop connect.

How to bind plug-slot pairs explicitly
======================================

.. @artefact interface connection

You can override the auto-connection
when more than one slot in the workshop could satisfy a plug
and |ws_markup| does not land on the one you wanted.
There are two paths:
edit the workshop definition for a persistent decision,
or use :command:`workshop connect` for a one-off change at runtime.

This is a different problem from same-interface plug conflicts,
where two plugs would compete over the same target.
For that case, see :ref:`how_resolve_plug_conflicts`,
which uses an inline :samp:`bind:` attribute to delegate one plug to another.


Detect the ambiguity
--------------------

When two SDKs in the workshop expose a slot of the same interface
that could satisfy the same plug,
the interface policy decides whether either of them
is reachable by auto-connection.
For the mount interface,
regular-SDK slots are excluded from auto-connection
and the plug ends up on the system SDK's default mount slot,
even though regular providers are in scope.
This guide uses three synthesized SDKs:
:samp:`provider-a` and :samp:`provider-b`
each expose a mount slot named :samp:`data`,
and :samp:`consumer` declares a mount plug named :samp:`feed`.

Launch the workshop and inspect the connections:

.. code-block:: console

   $ workshop launch dev
   $ workshop connections dev

     INTERFACE  PLUG               SLOT                 NOTES
     mount      -                  dev/provider-a:data  -
     mount      -                  dev/provider-b:data  -
     mount      dev/consumer:feed  dev/system:mount     -


:samp:`consumer:feed` ended up wired to the system SDK's default mount slot,
not to either regular provider,
because mount plugs auto-connect only to system-SDK slots.
Regular-SDK mount slots are never reached by auto-connection
and stay listed but unconnected
until a top-level :samp:`connections:` entry names the pair.
The result is a working workshop,
but probably not the one you intended.


Resolve persistently in the workshop definition
-----------------------------------------------

Add a top-level :samp:`connections:` entry to the workshop definition
that names the plug and the slot you want it to use:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 8-10

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: provider-a
     - name: provider-b
     - name: consumer

   connections:
     - plug: consumer:feed
       slot: provider-b:data


Refresh the workshop and verify:

.. code-block:: console

   $ workshop refresh dev
   $ workshop connections dev

     INTERFACE  PLUG               SLOT                 NOTES
     mount      -                  dev/provider-a:data  -
     mount      dev/consumer:feed  dev/provider-b:data  -


This decision is persistent;
re-launching the workshop or recreating it
applies the same pairing every time.


Resolve transiently with workshop connect
-----------------------------------------

For a one-off change, :command:`workshop connect` rewires a running workshop
without editing the workshop definition.
Pass the plug and the target slot explicitly:

.. code-block:: console

   $ workshop disconnect dev/consumer:feed
   $ workshop connect dev/consumer:feed dev/provider-a:data
   $ workshop connections dev

     INTERFACE  PLUG               SLOT                 NOTES
     mount      -                  dev/provider-b:data  -
     mount      dev/consumer:feed  dev/provider-a:data  manual


The :samp:`manual` note in the :samp:`Notes` column flags that the connection
came from a CLI invocation rather than the workshop definition
or the auto-connection mechanism.

The workshop definition on disk is unchanged,
but the runtime mark carries over across :command:`workshop refresh`:
:samp:`consumer:feed` stays connected to :samp:`provider-a:data`
until you rewire it again,
or until :command:`workshop disconnect --forget`
clears the mark and lets auto-connection apply.
A :command:`workshop remove` discards these runtime overrides,
so a subsequent :command:`workshop launch` starts from the definition.

For a decision that travels with the project
and survives :command:`workshop remove`,
edit the workshop definition instead.


See also
--------

Explanation:

- :ref:`exp_interface_concepts`
- :ref:`exp_plug_bindings`
- :ref:`exp_plugs_slots`


How-to guides:

- :ref:`how_design_interface_layout`
- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_refresh`
