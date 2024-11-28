.. _ref_workshop_list:

workshop list
-------------

List project workshops.

.. rubric:: Synopsis

.. code-block:: console

   workshop list [flags]

.. rubric:: Description


This command enumerates all workshops in the project, printing a compact list:

- Project:  absolute pathname of the project where this workshop belongs

- Workshop: workshop name, as set by its definition

- Status:   workshop status, such as *Off*, *Ready*, *Pending* and so on

- Notes:    internal remarks on the overall state of the workshop


The **--global** option lists all workshops from *all* projects in the system;
however, it doesn't include any that are *Off*.


Notes:

- For details of a single workshop, use **workshop info** instead


.. rubric:: Options


--global

   List workshops from all projects in the system



.. rubric:: Examples

.. code-block:: console
   
   # List the workshops in the current project directory
   workshop list
   
   # List the globally registered workshops
   workshop list --global
