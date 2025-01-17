.. _ref_workshop_remove:

workshop remove
---------------

Remove one or many workshops.

.. rubric:: Usage

.. code-block:: console

   $ workshop remove <WORKSHOP>... [flags]

.. rubric:: Description


This command removes the workshops listed as arguments. For each workshop, it:

- Checks that the workshop isn't 'Off' or 'Pending'
- Stops the workshop if it's not already 'Stopped'
- Deletes the workshop but preserves its definition

Notes:

- If any listed workshop is 'Off' or 'Pending', none are removed.

- To rebuild a removed workshop from scratch, use 'workshop launch'.

- For mount interface plugs,
  non-default sources set by 'workshop remount' aren't removed.


.. rubric:: Examples


Remove the 'nimble' and 'jazzy' workshops in the current project directory:

.. code-block:: console

   $ workshop remove nimble jazzy


The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop remove


