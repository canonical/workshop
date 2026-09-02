.. _ref_workshop_tasks:


.. meta::
   :description: Reference documentation for the 'workshop tasks' command

workshop tasks
--------------

.. @artefact workshop tasks

List tasks for a specific change.

.. rubric:: Usage

.. code-block:: console

   $ workshop tasks [<CHANGE ID>] [flags]

.. rubric:: Description

Any substantial operation on a workshop is a change that consists of tasks;
the command lists individual tasks that comprise a specific change.
For each task, it prints the following details:

- Status:    Reflects the task's progress and affects the status of the change
- Duration:  Tells how long the task has been running
- Summary:   Lists actions, affected SDKs and workshops, other information


Notes:

- The command may print additional log details for tasks that store them

- To investigate recent changes in a project, use "workshop changes" instead

.. rubric:: Examples

List the tasks under change ID 42:

.. code-block:: console

   $ workshop tasks 42

List the tasks under the most recent change to the project:

.. code-block:: console

   $ workshop tasks


.. rubric:: Flags

--no-headers

   Hide table headers.

