.. _ref_workshop_status:

.. meta::
   :description: Reference guide on workshop status changes, providing diagrams
                 that explain lifecycle states and transitions caused by various commands.

Workshop status diagrams
========================

During its lifecycle, a workshop goes through a number of states,
which we call *statuses* to distinguish them from SDK states.
The following partial diagrams represent each state
with the commands that cause the workshop to transition to a different status.

Off
---

Always the starting point,
where the workshop exists solely as a
:ref:`definition file <ref_workshop_definition>`
in the project directory;
there is no container yet.

.. mermaid::
   :alt: Off state
   :caption: Off state
   :align: center

   stateDiagram-v2
       OFF --> READY: launch
       OFF --> ERROR: launch (on error)
       OFF --> WAITING: launch --wait-on-error (on error)


Ready
-----

The workshop was successfully launched from the definition file;
its underlying container is linked to the project directory,
up and ready to do some work.

.. mermaid::
   :alt: Ready state
   :caption: Ready state
   :align: center

   stateDiagram-v2
       READY --> STOPPED: stop
       READY --> OFF: remove
       READY --> READY: remount
       READY --> READY: refresh
       READY --> ERROR: refresh (on error)
       READY --> WAITING: refresh --wait-on-error (on error)


Stopped
-------

The underlying container was stopped
but is still linked to the project directory.

.. mermaid::
   :alt: Stopped state
   :caption: Stopped state
   :align: center

   stateDiagram-v2
       STOPPED --> READY: start
       STOPPED --> STOPPED: remount


Waiting
-------

The workshop is paused
in the middle of a change
to allow for interactive debugging;
only a few commands will be accepted.

.. mermaid::
   :alt: Waiting state
   :caption: Waiting state
   :align: center

    stateDiagram-v2
        WAITING --> OFF: launch --abort
        WAITING --> READY: launch --continue
        WAITING --> READY: refresh --abort
        WAITING --> READY: refresh --continue


Pending
-------

An intermediate state
while the workshop is being updated or changing its status;
only a few commands will be accepted.


Error
-----

The workshop failed at some stage,
and its underlying container became nonoperational.

.. mermaid::
   :alt: Error state
   :caption: Error state
   :align: center

   stateDiagram-v2
       ERROR --> OFF: remove


See also
--------

Explanation:

- :ref:`exp_workshop_status`


Reference:

- :ref:`ref_cli`
