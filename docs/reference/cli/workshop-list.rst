.. _ref_workshop_list:

workshop list
-------------

List project workshops.

.. rubric:: Usage

.. code-block:: console

   $ workshop list [flags]

.. rubric:: Description


This command enumerates all workshops in the project, printing a compact list:

- Project:  Absolute pathname of the project where this workshop belongs

- Workshop: Workshop name, as set by its definition

- Status:   Workshop status, such as 'Off', 'Ready', 'Pending' and so on

- Notes:    Internal remarks on the overall state of the workshop


The '--global' option lists all workshops from all projects in the system;
however, it doesn't include any that are 'Off'.


Notes:

- For details of a single workshop, use 'workshop info' instead.


.. rubric:: Examples


List the workshops in the current project directory:

.. code-block:: console

   $ workshop list


List the globally registered workshops:

.. code-block:: console

   $ workshop list --global



.. rubric:: Flags


--global

   List workshops from all projects in the system.


