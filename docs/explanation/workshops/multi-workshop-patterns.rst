.. _exp_multi_workshop_patterns:

.. meta::
   :description: Explanation of the two architectural patterns for using
                 multiple workshops, the scenarios each pattern fits, and
                 the boundary between what stays shared and what stays
                 isolated.

Multi-workshop patterns
=======================

.. @artefact project
.. @artefact project workshops
.. @artefact workshop (container)
.. @artefact workshop definition
.. @artefact workshop name
.. @artefact workshop .lock
.. @artefact in-project SDK
.. @artefact tunnel interface

One workshop per project is the default,
but real work often pulls in more than one.
A monorepo holds a Go backend and a Node frontend,
each with its own toolchain.
A coding agent needs to run over a branch
without seeing sibling branches or unrelated host directories.
Two long builds must progress without blocking your editing.
A regression has to be confirmed against a new base image
without disturbing the working setup.

|ws_markup| supports these cases through two patterns
that combine the same primitives in different ways:
several workshops sharing one project directory,
or several project directories
each hosting one or more workshops of their own.


Two patterns
------------

The two patterns differ in where the project boundary falls:
within a single project directory,
or across several directories.
Both rely on the same underlying mechanics;
they're distinguished by intent and topology,
not by special CLI flags.


One project, multiple workshops
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Here, several workshops live in a single project directory
and share the project files mounted at :file:`/project/`.
Their definitions sit in the :file:`.workshop/` subdirectory,
each in its own file named after the workshop:

.. code-block:: none

   my-project/
   ├── .workshop/
   │   ├── frontend.yaml
   │   ├── backend.yaml
   │   └── common-tools/
   │       └── sdk.yaml
   ├── web/
   └── api/


Each workshop has its own base image,
its own SDK list,
and its own actions,
but they all see the same project files.
:ref:`In-project SDKs <exp_in_project_sdk>`,
stored as subdirectories of :file:`.workshop/`,
can be referenced by *any* workshop in the project
and provide a clean way to share custom tooling.

The workshops are isolated as LXD containers:
each has its own filesystem layers,
its own running processes,
and its own snapshot chain.
What's shared is the project content on the host,
not the runtime.
Direct interface connections between two workshops
are rejected by :command:`workshop connect`;
if your workshops need to talk to each other,
they may do so by bridging through the host
with two independent :ref:`tunnel <exp_tunnel_interface>` connections,
as described in :ref:`how_use_multiple_workshops`.

Pick this pattern when several components of one project
need different runtimes but the same source tree.


Multiple projects, one workshop each
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Here, you work on multiple related project directories.
Each project directory has its own
:file:`workshop.yaml`
(or its own :file:`.workshop/` subdirectory)
and its own :file:`.workshop.lock`.
Workshops in different projects are fully independent:
the project files they mount at :file:`/project/` differ,
the containers are separate,
and the workshop definitions can diverge freely.

In day-to-day work this pattern is most often realized
with :literalref:`git worktree <https://git-scm.com/docs/git-worktree>`,
which gives you several working trees of the same repository
in sibling directories.
Each worktree is a distinct project from |ws_markup|'s point of view,
gets its own project ID,
and can run a workshop that's named identically to its sibling
without any collision.
The :command:`workshop list` command,
invoked with :option:`!--global`,
shows the workshops across the projects currently tracked by |ws_markup|,
including their project paths.
:ref:`how_git_workshops` walks through the mechanics.

Pick this pattern when the parts have to stay separated:
different branches,
different snapshots of the codebase,
different base images for the same code,
or different agents whose blast radius should not overlap.


At a glance
~~~~~~~~~~~

.. list-table::
   :header-rows: 1
   :widths: 25 37 38

   * -
     - One directory, multiple workshops
     - Multiple directories, one workshop each

   * - Definition location
     - :file:`.workshop/<NAME>.yaml` per workshop
     - :file:`workshop.yaml`, or :file:`.workshop/`, per directory

   * - Mounted at :file:`/project/`
     - The same directory in every workshop
     - A different directory in each workshop

   * - Project files
     - Shared across workshops
     - Isolated per directory

   * - Containers and snapshots
     - One per workshop
     - One per workshop

   * - Cross-workshop networking
     - Bridge through the host with two tunnels
     - Same; each workshop reaches the others through the host
       like any other service

   * - Branch isolation
     - All workshops see whatever branch the project is on
     - Each directory pins its workshops to its branch

   * - Typical trigger
     - Polyglot components of one codebase
     - Parallel work on different branches or snapshots


Scenarios
---------

The patterns above answer most multiworkshop cases.
The list below names the recurring ones,
points to the pattern that fits,
and links to the how-to that covers the mechanics.

- *Polyglot project components.*
  A monorepo with parts in different languages or runtimes,
  for example a Go backend with a Node frontend,
  or Python data preparation alongside CUDA training and a Rust serving layer.
  Each component gets its own workshop in the same project;
  the project files are shared,
  the toolchains are not.

- *Shared internal tooling across components.*
  When several workshops in one project rely on the same linter,
  formatter, or generator,
  package it as an :ref:`in-project SDK <exp_in_project_sdk>`
  rather than duplicating the setup in each workshop definition.

- *Parallel work on independent branches.*
  Two unrelated features,
  a feature and a hotfix,
  or a feature and a pull-request review.
  Each branch is checked out in its own worktree,
  and each worktree runs its own workshop.
  Editing, building, and any agent activity in one worktree
  has no effect on the others.

- *A/B comparison of bases or SDK channels.*
  Two worktrees hold the same code
  but their workshop definitions differ in :samp:`base`
  or in SDK :samp:`channel`,
  letting you confirm a dependency bump,
  reproduce a version-specific bug,
  or evaluate a base image upgrade
  side by side with the working setup.

- *Confinement for code-running agents.*
  A coding agent in a worktree or a subdirectory
  sees only that slice of the repository;
  sibling worktrees and unrelated host directories stay out of reach.
  The workshop container adds a second isolation boundary
  beneath the worktree boundary,
  so even an agent invoked with relaxed permission flags
  cannot reach the host filesystem outside of what the workshop mounts.

- *Long-running task offload.*
  A build, training run, migration, or large refactor
  keeps progressing in one workshop
  while you continue editing in another.
  Use one project with multiple workshops
  if the task should see your current edits,
  or separate projects via worktrees
  if it should be pinned to a specific commit.

- *Local reproduction of CI failures.*
  Check out the failing commit in a dedicated worktree
  and define a workshop with the base image CI uses.
  Your everyday workshop stays on the working setup
  and continues to run.


Pattern combinations
--------------------

The two patterns combine.
A monorepo with :file:`.workshop/frontend.yaml`
and :file:`.workshop/backend.yaml`
can also be checked out in two worktrees,
each running both workshops.
This is useful when reviewing a pull request
on the monorepo's structure
while continuing development on the main branch:
the review worktree has its own frontend and backend workshops,
isolated from yours,
and :command:`workshop list` invoked with :option:`!--global`
shows all four at once.


See also
--------

Explanation:

- :ref:`exp_in_project_sdk`
- :ref:`exp_projects`
- :ref:`exp_tunnel_interface`
- :ref:`exp_workshop_concepts`

How-to guides:

- :ref:`how_forward_ports`
- :ref:`how_git_workshops`
- :ref:`how_use_multiple_workshops`
- :ref:`how_use_workshops_with_ai_agents`

Reference:

- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_list`
