.. _ref_workshop_connect:

workshop connect
----------------

Connect a plug to a slot

Synopsis
~~~~~~~~


This command connects a plug to a target slot
that is specified as the second argument or deduced from the context.

- If the second argument is omitted entirely, the target is assumed to be
  <WORKSHOP>/system:<PLUG>; <WORKSHOP> and <PLUG> come from the first argument

- If the second argument only names the slot itself, the target is
  <WORKSHOP>/system:<SLOT>; <WORKSHOP> comes from the first argument

- If the second argument only names the workshop and SDK, the target is
  <WORKSHOP>/<SDK>:<INTERFACE>;
  <INTERFACE> is the interface in the plug's definition.
  However, if there are several candidate slots that match the interface,
  the command fails

- If the target slot is compatible with the plug, the command attempts
  to connect them and returns the result


  Notes:

- To be compatible, the plug and the slot must use the same interface

- Multiple plugs can be connected to the same slot, but not vice versa

- The 'workshop connections' output will list the connection as 'manual'


.. code-block:: console

   workshop connect <WORKSHOP>/<SDK>:<PLUG> [<WORKSHOP>/<SDK>][:<SLOT>] [flags]

Options
~~~~~~~
--no-wait

   Return the change ID, don't wait for the operation to finish


