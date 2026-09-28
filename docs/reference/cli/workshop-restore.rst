.. _ref_workshop_restore:


.. meta::
   :description: Reference documentation for the 'workshop restore' command

workshop restore
----------------

.. @artefact workshop restore

Restore workshops to the state of the last launch or refresh.

.. rubric:: Usage

.. code-block:: console

   $ workshop restore <WORKSHOP>... [flags]

.. rubric:: Description

This command restores the container filesystem of the workshops listed
as arguments to the point of the last launch or refresh,
then resets the interface connections to default settings:

- Connections added at runtime with "workshop connect" are dropped,
  and the workshop returns to its definition's auto-connect defaults.

- A connection removed with "workshop disconnect" is re-established,
  whether or not "--forget" was used.

- A mount source set with "workshop remount" is reset
  to its default location.

Notes:

- The workshop must be "Ready" to be restored.

- Multiple workshops can be restored in a single command invocation;
  the operation is transactional, so if any workshop fails to restore,
  all are reverted.

- To update an existing workshop instead of reverting changes,
  use "workshop refresh".

.. rubric:: Examples

Restore the "nimble" and "jazzy" workshops in the current project directory:

.. code-block:: console

   $ workshop restore nimble jazzy

The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop restore


.. rubric:: Flags

--no-wait

   Return the change ID, don't wait for the operation to finish.

--verbose

   Combine stdout and stderr output from hooks.

