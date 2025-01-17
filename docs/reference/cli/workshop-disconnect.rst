.. _ref_workshop_disconnect:

workshop disconnect
-------------------

Disconnect a plug or a slot.

.. rubric:: Usage

.. code-block:: console

   $ workshop disconnect <WORKSHOP>/<SDK>:<PLUG OR SLOT> [<WORKSHOP>/<SDK>]:[<SLOT>] [flags]

.. rubric:: Description


This command disconnects a plug from its slot, or a slot from all its plugs.

- A single argument can be a fully qualified plug or slot reference;
  with two arguments, the first one is the plug, and the second one is the slot.

- If the second argument only names the slot itself, the target is
  <WORKSHOP>/system:<SLOT>; <WORKSHOP> comes from the first argument.

- If the second argument only names the workshop and SDK, the target is
  <WORKSHOP>/<SDK>:<INTERFACE>;
  <INTERFACE> is the interface in the plug's definition.


  Notes:

- After an auto-connected plug is thus disconnected,
  it is reconnected during 'workshop refresh'
  only if the '--forget' option was used with 'workshop disconnect'.


.. rubric:: Examples


Disconnect the 'mod-cache' mount interface plug of the 'go' SDK
under the 'nimble' workshop in the current project directory:

.. code-block:: console

   $ workshop disconnect nimble/go:mod-cache


A full version of the same command
that lists the target SDK ('system') and slot ('mount'):

.. code-block:: console

   $ workshop disconnect nimble/go:mod-cache nimble/system:mount


Disconnect all plugs connected to the 'mount' slot of the 'system' SDK
under the 'nimble' workshop in the current project directory:

.. code-block:: console

   $ workshop disconnect nimble/system:mount



.. rubric:: Flags


--forget

   Reconnect the plugs at 'workshop refresh' if auto-connected initially.


--no-wait

   Return the change ID, don't wait for the operation to finish.


