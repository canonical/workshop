.. _ref_workshop_tasks:

workshop tasks
--------------

List tasks for a specific change.

.. rubric:: Usage

.. code-block:: console

   $ workshop tasks <CHANGE ID> [flags]

.. rubric:: Description


Any substantial operation on a workshop is a change that consists of tasks;
the command lists individual tasks that comprise a specific change.
For each task, it prints the following details:

- ID:      Uniquely identifies the task within the change
- Status:  Reflects the task's progress and affects the change's status
- Spawn:   Tells when the task was started
- Ready:   Tells when the task was finished
- Summary: Lists actions, affected SDKs and workshops, other information


Notes:

- The command may print additional log details for tasks that store them

- To investigate recent changes in a project, use 'workshop changes' instead


.. rubric:: Examples


List the tasks under change ID 42:

.. code-block:: console

   $ workshop tasks 42


