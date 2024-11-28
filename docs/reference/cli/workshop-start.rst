.. _ref_workshop_start:

workshop start
--------------

Start one or many workshops.

.. rubric:: Synopsis

.. code-block:: console

   workshop start <WORKSHOP>... [flags]

.. rubric:: Description


This command activates the workshops listed as arguments. For each one, it:

- Makes sure the workshop was actually launched

- Activates the workshop for use and sets it to *Ready*


If multiple workshops are listed and an error occurs,
the operation is aborted and no workshops are started.


Notes:

- If a workshop is already started or wasn't yet launched, an error occurs

- When interrupted, the command attempts to gracefully revert its actions

- To stop a started workshop, use **workshop stop**


.. rubric:: Examples

.. code-block:: console
   
   # Start the 'nimble' and 'jazzy' workshops in the current project directory
   workshop start nimble jazzy
