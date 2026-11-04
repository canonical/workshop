.. _exp_changes_tasks:

.. meta::
   :description: Explanation of changes and tasks in Workshop, detailing how
                 state management operations are planned and executed as changes
                 consisting of ordered tasks.

Changes, tasks
==============

.. @artefact workshop state management

A *change* is a core concept of the workshop state management system.
Any long-running or invasive operation
(e.g., :ref:`launch <ref_workshop_launch>`)
that changes the state of a workshop
is planned and applied as a change,
which consists of specific tasks
that run in a predefined order.

.. @artefact project

A *task* is a small, independent piece of logic;
it could be mounting a project directory,
running a :ref:`hook <exp_sdk_hooks>`,
or starting a workshop container.
Most tasks are reversible.

Overall, this scheme provides granular control
over the state of a workshop;
the state management system uses it
to ensure the integrity of the workshop in the event of failure.
By default, a failed change restores the workshop
to its last operational state.

To explicitly revert to the state after the last successful launch or refresh,
use the :command:`workshop restore` command.
It restores the container filesystem from the most recent snapshot
and resets all connections and mounts to their defaults.
Unlike the automatic rollback during a failed refresh,
:command:`restore` is a deliberate user action
to discard all changes made to a workshop since it was last set up.


See also
--------

Explanation:

- :ref:`exp_sdk_hooks`


How-to guides:

- :ref:`how_debug_issues_workshops`


Reference:

- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_restore`
- :ref:`ref_workshop_tasks`
