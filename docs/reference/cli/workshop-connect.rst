.. _ref_workshop_connect:

workshop connect
----------------

Connect a plug to a slot.

.. rubric:: Usage

.. code-block:: console

   $ workshop connect <WORKSHOP>/<SDK>:<PLUG> [<WORKSHOP>/<SDK>][:<SLOT>] [flags]

.. rubric:: Description


This command connects a plug to a target slot
that is specified as the second argument or deduced from the context.

- If the second argument is omitted entirely, the target is assumed to be
  <WORKSHOP>/system:<PLUG>; <WORKSHOP> and <PLUG> come from the first argument.

- If the second argument only names the slot itself, the target is
  <WORKSHOP>/system:<SLOT>; <WORKSHOP> comes from the first argument.

- If the second argument only names the workshop and SDK, the target is
  <WORKSHOP>/<SDK>:<INTERFACE>;
  <INTERFACE> is the interface in the plug's definition.
  However, if there are several candidate slots that match the interface,
  the command fails.

- If the target slot is compatible with the plug, the command attempts
  to connect them and returns the result.


  Notes:

- To be compatible, the plug and the slot must use the same interface.

- Multiple plugs can be connected to the same slot, but not vice versa.

- The 'workshop connections' output will list the connection as 'manual'.


.. rubric:: Examples


Connect the 'mod-cache' mount interface plug of the 'go' SDK
under the 'nimble' workshop in the current project directory:

.. code-block:: console

   $ workshop connect nimble/go:mod-cache :mount


A full version of the command that also lists the target SDK ('system'):

.. code-block:: console

   $ workshop connect nimble/go:mod-cache nimble/system:mount



.. rubric:: Flags


--no-wait

   Return the change ID, don't wait for the operation to finish.


