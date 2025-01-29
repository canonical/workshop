.. _ref_workshop_status:

Workshop status diagrams
========================

.. @artefact workshop status
.. @artefact project

During its life-cycle, a workshop goes through a number of states,
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
       OFF --> PENDING: launch --wait-on-error (on error)


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
       READY --> PENDING: refresh --wait-on-error (on error)


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


Pending
-------

The workshop is being updated or changing its status;
only a few commands will be accepted,
and the container itself is non-operational.

.. mermaid::
   :alt: Pending state
   :caption: Pending state
   :align: center

    stateDiagram-v2
        PENDING --> OFF: launch --abort
        PENDING --> READY: launch --continue
        PENDING --> READY: refresh --abort
        PENDING --> READY: refresh --continue


Error
-----

The workshop failed at some stage,
and its underlying container became non-operational.

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
