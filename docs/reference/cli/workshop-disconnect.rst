.. _ref_workshop_disconnect:

workshop disconnect
-------------------

Disconnect a plug or a slot

Synopsis
~~~~~~~~


This command disconnects a plug from its slot, or a slot from all its plugs.

- A single argument can be a fully qualified plug or slot reference;
  with two arguments, the first one is the plug, and the second one is the slot

- If the second argument only names the slot itself, the target is
  <WORKSHOP>/system:<SLOT>; <WORKSHOP> comes from the first argument

- If the second argument only names the workshop and SDK, the target is
  <WORKSHOP>/<SDK>:<INTERFACE>;
  <INTERFACE> is the interface in the plug's definition


  Notes:

- After an auto-connected plug is thus disconnected,
  it is reconnected during 'workshop refresh'
  only if the '--forget' option was used with 'workshop disconnect'


.. code-block:: console

   workshop disconnect <WORKSHOP>/<SDK>:<PLUG OR SLOT> [<WORKSHOP>/<SDK>]:[<SLOT>] [flags]

Options
~~~~~~~
--forget

   Reconnect the plugs at 'workshop refresh' if auto-connected initially


--no-wait

   Return the change ID, don't wait for the operation to finish


