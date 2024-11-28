.. _ref_workshop_connections:

workshop connections
--------------------

List interface connections.

.. rubric:: Synopsis

.. code-block:: console

   workshop connections [<WORKSHOP>] [flags]

.. rubric:: Description


This command lists the connections between interface plugs and slots
for the entire project or a single workshop within it.
Each line represents a connection between a plug and a slot via an interface;
additional notes, including specific plug bindings, are provided as needed.


Notes:

- The output lists connections created with **workshop connect** as *manual*

- The **--all** option needn't be used with an argument;
  if a workshop is supplied, disconnected plugs are also listed


.. rubric:: Options


--all

   Include disconnected plugs in the output.



.. rubric:: Examples

.. code-block:: console
   
   # List connections for the workshop 'nimble' in the current project directory
   workshop connections nimble
   
   # List connections for all workshops in the current project directory:
   workshop connections
