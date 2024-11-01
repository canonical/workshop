.. _ref_workshop_tasks:

workshop tasks
--------------

List tasks for a specific change

Synopsis
--------


Any substantial operation on a workshop is a *change* that consists of *tasks*;
the command lists individual tasks that comprise a specific change.
For each task, it prints the following details:

- ID:      uniquely identifies the task within the change
- Status:  reflects the task's progress and affects the change's status
- Spawn:   tells when the task was started
- Ready:   tells when the task was finished
- Summary: lists actions, affected SDKs and workshops, other information


Notes:

- The command may print additional log details for tasks that store them

- To investigate recent changes in a project, use 'workshop changes' instead


.. code-block:: console

   workshop tasks <CHANGE ID> [flags]

