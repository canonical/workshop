.. _ref_workshop_stop:

workshop stop
-------------

Stop one or many workshops.

.. rubric:: Usage

.. code-block:: console

   $ workshop stop <WORKSHOP>... [flags]

.. rubric:: Description


This command deactivates the workshops listed as arguments. For each one, it:

- Makes sure the workshop was actually started or is already stopped

- Deactivates the workshop and sets it to 'Stopped'


If multiple workshops are listed and an error occurs,
the operation is aborted and no workshops are stopped.


Notes:

- If a workshop wasn't yet started or even launched, an error occurs.

- When interrupted, the command attempts to gracefully revert its actions.

- To start a stopped workshop, use 'workshop start'.


.. rubric:: Examples


Stop the nimble and jazzy workshops in the current project directory:

.. code-block:: console

   $ workshop stop nimble jazzy


The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop stop


