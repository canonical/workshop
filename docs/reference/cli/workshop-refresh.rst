.. _ref_workshop_refresh:

workshop refresh
----------------

Update workshops according to their definitions.

.. rubric:: Synopsis

.. code-block:: console

   workshop refresh [--abort|--continue|--wait-on-error] <WORKSHOP>[/<SDK>]... [flags]

.. rubric:: Description


This command updates the workshops listed as arguments by going over their
definitions once again. For each workshop, it:

- Saves the working state of the workshop

- Checks the workshop definition and identifies any updates required

- Retrieves the updated components

- Applies and verifies the changes to the workshop

- Restores the working state of the workshop


The '--wait-on-error' option pauses the refresh if an error occurs.
Thus, you can fix the error and resume the operation or abort and revert it.
This option can only be used with a single workshop.
If multiple workshops are listed and an error occurs,
the operation is aborted and reverted for *all* of them.


Notes:

- The workshop must be *Ready* to be refreshed

- To construct a newly defined workshop, use **workshop launch** instead

- Throughout the refresh, all affected workshops remain *Pending*

- If the refresh removes an SDK from the workshop, the SDK state isn't saved

- Updated and newly added SDKs are installed in alphabetical order

- For content interface plugs, mounts the last source
  set by **workshop remount**, if any

- If the optional <SDK> is supplied,
  the operation is limited to this SDK;
  currently, it can only be **hack**


.. rubric:: Options


--abort

   Abort the previously paused operation, reverting any changes.


--continue

   Continue the previously paused operation.


--wait-on-error

   Pause the operation on error; to resume, use **--continue** or **--abort**.



.. rubric:: Examples

.. code-block:: console
   
   # Refresh the 'nimble' and 'jazzy' workshops in the current project directory
   workshop refresh nimble jazzy
   
   # Refresh 'nimble', but stop on any errors (won’t accept multiple workshops)
   workshop refresh nimble --wait-on-error
   
   # After 'nimble' refresh stopped on error, abort the operation
   workshop refresh nimble --abort
   
   # After 'nimble' refresh stopped on error and the workshop was fixed,
   # continue the operation
   workshop refresh nimble --continue
   
   # Refresh the hack SDK under 'nimble'
   workshop refresh nimble/hack
