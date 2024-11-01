.. _ref_workshop_connections:

workshop connections
--------------------

List interface connections

Synopsis
--------


This command lists the connections between interface plugs and slots
for the entire project or a single workshop within it.
Each line represents a connection between a plug and a slot via an interface;
additional notes, including specific plug bindings, are provided as needed.


Notes:

- The output lists connections created with 'workshop connect' as 'manual'

- The '--all' option needn't be used with an argument;
  if a workshop is supplied, disconnected plugs are also listed


.. code-block:: console

   workshop connections [<WORKSHOP>] [flags]


Options
-------

.. code-block:: console

      --all   Include disconnected plugs in the output.

