.. _ref_workshop_refresh:


.. meta::
   :description: Reference documentation for the 'workshop refresh' command

workshop refresh
----------------

.. @artefact workshop refresh

Update workshops according to their definitions.

.. rubric:: Usage

.. code-block:: console

   $ workshop refresh <WORKSHOP>... [flags]

.. rubric:: Description


This command updates the workshops listed as arguments. For each workshop,
it checks the workshop definition and applies any required updates
to the base image, SDKs, and interface connections:

- Connections added at runtime with "workshop connect" are preserved,
  as long as their plugs and slots still exist in the new definition.

- A connection removed with "workshop disconnect" without "--forget"
  stays disconnected after refresh.

The "--wait-on-error" option pauses the refresh if an error occurs.
Thus, you can fix the error and resume the operation or abort and revert it.
This option can only be used with a single workshop.
If multiple workshops are listed and an error occurs,
the operation is aborted and reverted for all of them.
Also, if you change the workshop definition while fixing the error,
you must abort the operation and restart from scratch.

Notes:

- The workshop must be "Ready" to be refreshed. Throughout
  the refresh, all affected workshops remain unavailable for other changes.

- Updated and newly added SDKs are installed in the order
  they are listed in the workshop definition.

- To construct a newly defined workshop, use "workshop launch" instead.



.. rubric:: Examples


Refresh the "nimble" and "jazzy" workshops in the current project directory:

.. code-block:: console

   $ workshop refresh nimble jazzy


The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop refresh


Refresh workshop, but pause on any errors (won't accept multiple workshops):

.. code-block:: console

   $ workshop refresh --wait-on-error


After refresh paused on error, abort the operation:

.. code-block:: console

   $ workshop refresh --abort


After refresh paused on error and the workshop was fixed,
continue the operation:

.. code-block:: console

   $ workshop refresh --continue



.. rubric:: Flags


--abort

   Abort the previously paused operation, reverting any changes.


--continue

   Continue the previously paused operation.


--no-wait

   Return the change ID, don't wait for the operation to finish.


--verbose

   Combine stdout and stderr output from hooks.


--wait-on-error

   Pause the operation on error; to resume, use "--continue" or "--abort".




