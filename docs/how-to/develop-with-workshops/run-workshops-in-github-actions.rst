.. meta::
   :description: How-to guide on launching workshops in GitHub Actions
                 using the launch-workshop action on hosted runners,
                 with support for matrix builds and SDK caching.

.. _how_run_workshops_in_github_actions:

How to run workshops in GitHub Actions
======================================

.. @artefact workshop launch
.. @artefact workshop exec

The `launch-workshop <https://github.com/canonical/launch-workshop>`_ action
installs |ws_markup| on a GitHub-hosted runner
and launches an ephemeral workshop for the duration of a job.
Use it to run your project's tests, builds, or other tasks
inside the same workshop you use locally,
without standing up a self-hosted runner.

If you'd rather run jobs on your own hardware,
use a workshop as a self-hosted runner instead;
see :ref:`how_run_github_actions_locally`.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- A GitHub repository with Actions enabled

- A workshop definition committed to the repository,
  at :file:`workshop.yaml` or :file:`.workshop/<NAME>.yaml`


Add the action to a workflow
----------------------------

The smallest useful workflow
checks out the repository,
launches the default workshop,
and runs a command inside it:

.. code-block:: yaml
   :caption: .github/workflows/test.yaml

   on:
     pull_request:
     push:
       branches: [main]
   jobs:
     test:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v6

         - uses: canonical/launch-workshop@v1

         - run: workshop exec -- pytest


If the repository contains a single :file:`workshop.yaml`
at the project root,
the action launches it automatically.
For repositories with several workshops under :file:`.workshop/`,
set the :samp:`workshop` input to the one you need.

For production use, pin the action to a specific commit SHA.
The :samp:`@v1` shorthand shown above tracks the latest :samp:`v1.*`
release and can be moved between versions;
:samp:`@main` is even less stable.


Test across multiple workshops
------------------------------

To test a project against several workshops in parallel,
parameterize the :samp:`workshop` input with a matrix:

.. code-block:: yaml
   :caption: .github/workflows/matrix.yaml
   :emphasize-lines: 6,12

   jobs:
     test:
       runs-on: ubuntu-latest
       strategy:
         matrix:
           workshop: [dev-jammy, dev-noble]
       steps:
         - uses: actions/checkout@v6

         - uses: canonical/launch-workshop@v1
           with:
             workshop: ${{ matrix.workshop }}

         - run: workshop run "$WS" unit-tests
           env:
             WS: ${{ matrix.workshop }}


This pattern fits well for testing the same project
against different Ubuntu releases,
different SDK channels,
or any other axis you encode in the workshop name.


.. _how_run_workshops_in_github_actions_cache:

Cache SDK data across runs
--------------------------

Some SDKs expose mount plugs that can be persisted between workflow runs,
such as a package cache or a build cache.
List the available plugs in your local workshop
with :command:`workshop connections`:

.. code-block:: console

   $ workshop connections --all

     INTERFACE  PLUG      SLOT          NOTES
     mount      uv:cache  system:mount  -


Pass the matching :samp:`<SDK>:<PLUG>` lines to the :samp:`cache` input,
one per line:

.. code-block:: yaml
   :caption: .github/workflows/test.yaml
   :emphasize-lines: 4-7

   steps:
     - uses: canonical/launch-workshop@v1
       with:
         cache: |
           go:mod-cache
           rust:cargo-registry
           uv:cache


Each listed plug is mounted from a GitHub-managed cache,
keyed by the SDK and plug name.
Not every SDK defines cacheable plugs;
check the SDK's documentation when in doubt.


Inputs
------

The action exposes the following inputs:

.. list-table::
   :header-rows: 1

   * - Input
     - Description

   * - :samp:`channel`
     - Channel used to install the |ws_markup| snap.
       Optional; defaults to :samp:`latest/stable`.

   * - :samp:`revision`
     - Specific revision of the |ws_markup| snap to install.
       Optional.

   * - :samp:`project`
     - Directory containing a workshop to launch.
       Defaults to the repository root.

   * - :samp:`workshop`
     - Name of the workshop to launch.
       Required if the project defines several workshops.

   * - :samp:`cache`
     - Mount plugs to cache across runs,
       one :samp:`<SDK>:<PLUG>` entry per line.


.. _how_run_workshops_in_github_actions_security:

Security considerations
-----------------------

When integrating the action into your workflows:

- Pin the action to a commit SHA
  so a compromised tag cannot push unreviewed code into your workflows.


See also
--------

How-to guides:

- :ref:`how_git_workshops`
- :ref:`how_run_github_actions_locally`
