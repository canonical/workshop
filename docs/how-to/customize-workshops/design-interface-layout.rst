.. _how_design_interface_layout:
.. _how_connect_plug_slot_pairs_explicitly:

.. meta::
   :description: Shape a workshop's interface topology with explicit connection
                 entries in the workshop definition: survey what auto-connects,
                 wire a consumer to a specific provider, graft a missing plug,
                 or rewire a running workshop with workshop connect.

How to design the interface layout of a workshop
================================================

.. @tests in tests/docs-how-to/design-interface-layout/task.yaml

.. @artefact interface connection
.. @artefact workshop definition

You can shape the topology of a workshop
by writing explicit plug-to-slot connections in the workshop definition.
Use it when several SDKs in the workshop expose or consume the same interface
and you want to be specific about which provider satisfies which consumer,
when auto-connection lands a plug on a slot you did not intend,
or when a consumer SDK ships no plug for a capability you want it to use.
You need a workshop definition under :file:`.workshop/`
that lists at least two SDKs, one with a slot and one with a matching plug.

This is a different problem from same-interface plug conflicts,
where two plugs would compete over the same target.
For that case, see :ref:`how_resolve_plug_conflicts`,
which uses an inline :samp:`bind:` attribute to delegate one plug to another.


Survey the plugs and slots in scope
-----------------------------------

Launch the workshop once to see what |ws_markup| connects on its own.
The examples here use three SDKs:
:samp:`provider-a` and :samp:`provider-b`
each expose a mount slot named :samp:`data`,
and :samp:`consumer` declares a mount plug named :samp:`feed`.

.. code-block:: console

   $ workshop launch dev
   $ workshop connections dev

     INTERFACE  PLUG               SLOT                 NOTES
     mount      -                  dev/provider-a:data  -
     mount      -                  dev/provider-b:data  -
     mount      dev/consumer:feed  dev/system:mount     -


The output lists every plug and slot in the workshop,
the slot each plug is connected to (if any),
and any notes on the connection.
:samp:`consumer:feed` landed on the system SDK's default mount slot,
not on either regular provider,
because mount plugs auto-connect to system SDK slots by default.
The :samp:`provider-a:data` and :samp:`provider-b:data` slots
stay listed but unconnected:
regular SDK mount slots are not reached by auto-connection by default,
even when a matching plug is in scope.
The result is a working workshop, but probably not the one you intended.
A regular SDK slot becomes usable
only when a top-level :samp:`connections:` entry names the pair.


Wire a consumer to a specific provider
--------------------------------------

Add a top-level :samp:`connections` list to the workshop definition,
pairing the plug with the slot you want it to use:

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


Each entry uses the :samp:`<SDK-NAME>:<NAME>` form on both sides.
After :command:`workshop refresh`,
|ws_markup| applies the listed pairing
regardless of what other slots could have matched it:

.. code-block:: console

   $ workshop refresh dev
   $ workshop connections dev

     INTERFACE  PLUG               SLOT                 NOTES
     mount      -                  dev/provider-a:data  -
     mount      dev/consumer:feed  dev/provider-b:data  -


This decision is persistent;
re-launching the workshop or recreating it
applies the same pairing every time.
To inspect the resolved mount details, run :command:`workshop info dev`,
which lists each connected plug along with the source path the slot exposed
and the target path inside the workshop.


Graft a missing plug onto a consumer SDK
----------------------------------------

Suppose a consumer SDK ships no plug for the capability you want it to use.
The workshop definition can add one:
declare the plug under the SDK's entry in :samp:`sdks`,
then connect it as before.
This example pairs :samp:`provider-sdk`,
which exposes a mount slot named :samp:`bin`,
with :samp:`consumer-sdk`, which ships no matching plug:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 5-9, 11-13

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: provider-sdk
     - name: consumer-sdk
       plugs:
         tools:
           interface: mount
           workshop-target: /home/workshop/.local/share/tools

   connections:
     - plug: consumer-sdk:tools
       slot: provider-sdk:bin


This grafts a new plug onto :samp:`consumer-sdk`
without modifying the SDK itself.
The publisher does not need to ship every plug
their users might want;
the workshop author can add the missing piece locally.


Rewire a running workshop with workshop connect
-----------------------------------------------

For a one-off change, :command:`workshop connect` rewires a running workshop
without editing the workshop definition.
Pass the plug and the target slot explicitly:

.. code-block:: console

   $ workshop disconnect dev/consumer:feed
   $ workshop connect dev/consumer:feed dev/provider-a:data
   $ workshop connections dev

     INTERFACE  PLUG               SLOT                 NOTES
     mount      -                  dev/provider-b:data  -
     mount      dev/consumer:feed  dev/provider-a:data  manual


The :samp:`manual` note in the :samp:`NOTES` column flags that the connection
came from a CLI invocation rather than the workshop definition
or the auto-connection mechanism.

The workshop definition on disk is unchanged,
but the runtime mark carries over across :command:`workshop refresh`:
:samp:`consumer:feed` stays connected to :samp:`provider-a:data`
until you rewire it again,
or until you run :command:`workshop disconnect --forget`,
after which the next :command:`workshop refresh` re-applies auto-connection.
Running :command:`workshop remove` discards these runtime overrides,
so a subsequent :command:`workshop launch` starts from the definition.

For a decision that travels with the project
and survives :command:`workshop remove`,
edit the workshop definition instead.


See also
--------

Explanation:

- :ref:`exp_interface_concepts`
- :ref:`exp_plug_bindings`
- :ref:`exp_plugs_slots`


How-to guides:

- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_info`
- :ref:`ref_workshop_refresh`
