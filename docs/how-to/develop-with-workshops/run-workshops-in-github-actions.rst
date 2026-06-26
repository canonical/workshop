.. meta::
   :description: How-to guide on launching workshops in GitHub Actions
                 using the launch-workshop action on hosted runners,
                 with support for matrix builds and SDK caching.

.. _how_run_workshops_in_github_actions:

How to run workshops in GitHub Actions
======================================

.. @artefact workshop launch
.. @artefact workshop exec
.. @tests in tests/docs-how-to/run-workshops-in-github-actions/task.yaml

The `launch-workshop <https://github.com/canonical/launch-workshop>`_ action
installs |ws_markup| on a GitHub-hosted runner
and launches an ephemeral workshop for the duration of a job.
Use it to run your project's tests, builds, or other tasks
inside the same workshop you use locally,
without standing up a self-hosted runner.

For a complete, runnable example,
see the `workshop-ci-demo <https://github.com/akcano/workshop-ci-demo>`__ repository.

If you'd rather run jobs on your own hardware,
use a workshop as a self-hosted runner instead;
see :ref:`how_run_github_actions_locally`.


Prerequisites
-------------

Before getting started, ensure you have:

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

         - uses: canonical/launch-workshop@v0
           with:
             token: ${{ secrets.GITHUB_TOKEN }}

         - run: workshop exec -- pytest


The action installs |ws_markup|
from the public :samp:`canonical/workshop` repository,
so the built-in :samp:`GITHUB_TOKEN` is all the :samp:`token` input needs;
no personal access token or repository secret is required.
To install |ws_markup| from a private mirror instead,
supply a token with read access to that repository,
stored as an Actions secret.

If the repository contains a single :file:`workshop.yaml`
at the project root,
the action launches it automatically.
For repositories with several workshops under :file:`.workshop/`,
set the :samp:`workshop` input to the one you need.

For production use, pin the action to a specific commit SHA.
The :samp:`@v0` shorthand shown above tracks the latest :samp:`v0.*`
release and can be moved between versions;
:samp:`@main` is even less stable.


Test across multiple workshops
------------------------------

To test a project against several workshops in parallel,
parameterize the :samp:`workshop` input with a matrix:

.. code-block:: yaml
   :caption: .github/workflows/matrix.yaml
   :emphasize-lines: 6,13

   jobs:
     test:
       runs-on: ubuntu-latest
       strategy:
         matrix:
           workshop: [dev-jammy, dev-noble]
       steps:
         - uses: actions/checkout@v6

         - uses: canonical/launch-workshop@v0
           with:
             token: ${{ secrets.GITHUB_TOKEN }}
             workshop: ${{ matrix.workshop }}

         - run: workshop run "$WS" unit-tests
           env:
             WS: ${{ matrix.workshop }}


This pattern fits well for testing the same project
against different Ubuntu releases,
different SDK channels,
or any other axis you encode in the workshop name.


Cache SDK data across runs
--------------------------

Some SDKs expose mount plugs that can be persisted between workflow runs,
such as a package cache or a build cache.
List the available plugs in your local workshop
with :command:`workshop connections`:

.. code-block:: console

   $ workshop connections --all

     INTERFACE  PLUG              SLOT          NOTES
     mount      python:pip-cache  system:mount  -


Pass the matching :samp:`<SDK>:<PLUG>` lines to the :samp:`cache` input,
one per line:

.. code-block:: yaml
   :caption: .github/workflows/test.yaml
   :emphasize-lines: 5-9

   steps:
     - uses: canonical/launch-workshop@v0
       with:
         token: ${{ secrets.GITHUB_TOKEN }}
         cache: |
           cargo:git
           cargo:registry
           go:mod-cache
           python:pip-cache


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

   * - :samp:`token`
     - Token used to download |ws_markup| from :samp:`canonical/workshop`.
       The built-in :samp:`GITHUB_TOKEN` works because that repository is public.
       Required.

   * - :samp:`version`
     - |ws_markup| version or range of versions. Defaults to :samp:`latest`.

   * - :samp:`project`
     - Directory containing a workshop to launch.
       Defaults to the repository root.

   * - :samp:`workshop`
     - Name of the workshop to launch.
       Required if the project defines several workshops.

   * - :samp:`cache`
     - Mount plugs to cache across runs,
       one :samp:`<SDK>:<PLUG>` entry per line.


Security considerations
-----------------------

When integrating the action into your workflows:

- Pin the action to a commit SHA
  so a compromised tag cannot push unreviewed code into your workflows.

- The built-in :samp:`GITHUB_TOKEN` is sufficient
  for the public :samp:`canonical/workshop` repository,
  so no long-lived personal access token is required.

- If you install |ws_markup| from a private mirror,
  store its access token as an Actions secret,
  prefer a fine-grained token limited to read access,
  and rotate it immediately
  if it ever appears in logs, chat history, or any other shared transcript.


See also
--------

How-to guides:

- :ref:`how_git_workshops`
- :ref:`how_run_github_actions_locally`
