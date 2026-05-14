.. _how_design_capability_topology:

.. meta::
   :description: Wire SDKs together in a workshop by adding explicit
                 connection entries to the workshop definition, choosing
                 which provider satisfies a given consumer.

How to design the interface layout of a workshop
================================================

.. @artefact interface connection
.. @artefact workshop definition

You can shape the topology of a workshop
by writing explicit plug-to-slot connections in the workshop definition.
Use it when several SDKs in the workshop expose or consume the same interface
and you want to be specific about which provider satisfies which consumer,
or when the auto-connection mechanism cannot make the choice on its own.


Prerequisites
-------------

You need a workshop definition under :file:`.workshop/`
that lists at least two SDKs, one with a slot and one with a matching plug.
This guide uses a synthesized pair:
:samp:`provider-sdk` exposes a mount slot named :samp:`bin`
that publishes a directory inside the SDK,
and :samp:`consumer-sdk` declares a mount plug named :samp:`tools`
that targets a path under the workshop user's home.


Survey the plugs and slots in scope
-----------------------------------

Launch the workshop once to see what |ws_markup| connects on its own:

.. code-block:: console

   $ workshop launch dev
   $ workshop connections dev


The output lists every plug in the workshop,
the slot it is connected to (if any),
and any notes on the connection.
A plug that has no slot in the :samp:`SLOT` column
is unconnected;
either no slot matches it,
or several slots match and auto-connection has refused to choose.


Wire a consumer to a specific provider
--------------------------------------

Add a top-level :samp:`connections` list to the workshop definition,
pairing the plug with the slot you want it to use:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 7-9

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: provider-sdk
     - name: consumer-sdk

   connections:
     - plug: consumer-sdk:tools
       slot: provider-sdk:bin


Each entry uses the :samp:`<SDK-NAME>:<NAME>` form on both sides.
After :command:`workshop refresh`,
|ws_markup| applies the listed pairing
regardless of what other slots could have matched it.

Confirm with :command:`workshop connections`:

.. code-block:: console

   $ workshop refresh dev
   $ workshop connections dev

     INTERFACE  PLUG                       SLOT                    NOTES
     mount      dev/consumer-sdk:tools     dev/provider-sdk:bin
     ...


To inspect the resolved mount details, use :command:`workshop info`:

.. code-block:: console

   $ workshop info dev


The output lists each connected plug along with the source path
the slot exposed and the target path inside the workshop.


Graft a missing plug onto a consumer SDK
----------------------------------------

If the consumer SDK does not ship a plug for the capability you want it to use,
the workshop definition can add one.
Declare the plug under the SDK's entry in :samp:`sdks`,
then connect it as before:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 5-8, 11-13

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


See also
--------

Explanation:

- :ref:`exp_interface_concepts`
- :ref:`exp_plugs_slots`


How-to guides:

- :ref:`how_bind_plug_slot_pairs_explicitly`
- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_info`
- :ref:`ref_workshop_refresh`
