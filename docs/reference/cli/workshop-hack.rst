.. _ref_workshop_hack:

workshop hack
-------------

Edit the hack SDK and graft it onto the workshop

Synopsis
~~~~~~~~


This command opens the default text editor to configure the 'hack' SDK
and immediately installs it in the specified workshop,
enabling rapid experiments and tweaks at the SDK level.

If <HOOK> isn't specified, the command opens the SDK definition file.
Setting the <HOOK> value opens the respective hook file:

- 'check-health'
- 'restore-state'
- 'save-state'
- 'setup-base'


Saving and exiting causes a refresh,
which installs the updated 'hack' SDK in the workshop.

The '--drop' and '--restore' options stash the 'hack' SDK,
reversing the changes, and quickly restore it to the workshop.


Notes:

- The 'hack' SDK doesn't appear in the workshop definition
  and cannot include build-time data such as parts

- In addition to hooks, the 'hack' SDK can use interfaces,
  define plugs, slots, connections and bindings

- You can partially refresh the workshop, targeting the 'hack' SDK
  with the 'workshop refresh <WORKSHOP>/hack' command


.. code-block:: console

   workshop hack [--drop|--restore] <WORKSHOP> [setup-base|save-save|restore-state|check-health] [flags]

Options
~~~~~~~
--drop

   Drop the hack SDK from the workshop.


--restore

   Return the previously dropped SDK to the workshop.


