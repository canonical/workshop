.. meta::
   :description: How-to guide on running GitHub Actions locally
                 using the github-runner SDK,
                 enabling local testing, debugging,
                 and development of CI/CD workflows.

.. _how_run_github_actions_locally:

How to run GitHub Actions locally
=================================

.. @tests not applicable: requires GitHub Actions runner registration

Running GitHub Actions locally
provides a powerful way to develop, test, and debug CI/CD workflows.
This approach offers faster feedback loops and greater control
over the execution environment while maintaining compatibility
with your existing GitHub Actions workflows.

.. @artefact SDK

The :samp:`github-runner` SDK lets a workshop act as a `just-in-time runner
<https://docs.github.com/en/actions/reference/security/secure-use#using-just-in-time-runners>`_
for `GitHub workflow jobs
<https://docs.github.com/en/actions/concepts/workflows-and-actions/workflows>`_.

Running jobs locally makes a few things easier:

- Inspecting logs and other files after a failed run
- Interactive debugging, profiling, and tracing
- Testing with new, unusual, or expensive hardware
- Shorter feedback loops while ensuring consistency with remote runs


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- A GitHub account with admin permissions on the target repository,
  or "self-hosted runners" permission for organization-level runners.


Set up the workshop
-------------------

To run GitHub Actions locally,
create or update your workshop definition
to include the :samp:`github-runner` SDK:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 4

   name: ci
   base: ubuntu@24.04
   sdks:
     - name: github-runner


This installs the official `Runner <https://github.com/actions/runner>`_ client
and an unofficial helper script named :samp:`github-runner`.

.. @artefact workshop launch
.. @artefact workshop refresh

Don't forget to launch or refresh the workshop.

.. note::

   GitHub-hosted runners have a lot of preinstalled software,
   most of which isn't included in workshops by default.
   If a workflow-based run fails because of missing software,
   we recommend installing it as part of the workflow.
   This makes local and remote runs more consistent.
   Some actions (e.g., `setup-python <https://github.com/actions/setup-python>`_)
   provide additional features like caching.

   Some tools (notably Docker) aren't as easy to install during a job,
   but are available as SDKs.
   Others (such as :program:`yq`) are useful for development in addition to CI.
   These can be sketched into an SDK alongside :samp:`github-runner`;
   refer to the `See also`_ section for details.


Configure authorization
-----------------------

An important step is to authorize the :samp:`github-runner` SDK
to access your GitHub repositories or organization.


Choose a repository or organization
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

First, choose a repository or organization for the runner.

Admin-level permissions are required to add a runner to a repository.
Runners have access to `secrets
<https://docs.github.com/en/actions/concepts/security/secrets>`_,
so these permissions should be carefully guarded.
Users without admin rights can fork the repository
and test their workflows in the fork.

Another option is to add a runner to an organization,
which doesn't require admin rights on the organization,
but does grant access to organization secrets.
Proceed at your own risk.


Share permissions
~~~~~~~~~~~~~~~~~

The :samp:`github-runner` script needs the above permissions
to add the runner on your behalf.
When it runs for the first time,
it will request authorization
using a one-time code.

To limit the SDK's access to the necessary repositories,
the request is mediated by a `GitHub App
<https://docs.github.com/en/apps/using-github-apps/about-using-github-apps>`__
provided courtesy of the Workshop team.
By default, the SDK can't access any repository or organization on your behalf.

To grant access,
navigate to the `GitHub App for github-runner
<https://github.com/apps/test-app-jonathan-conder-1>`_
and install it.
Once installed,
the repositories it has access to can be configured at any time.
If the workshop or host machine is compromised,
the App should be uninstalled to limit the damage.

- For individuals,
  the App should be installed on a personal account
  and granted access to the required repositories.

- For organizations,
  the App must be installed on the organization.
  After adding a runner to the organization,
  workflows can use it
  even if the App is denied access to the relevant repository.
  Alternatively,
  the runner can be added to individual repositories within the organization.
  The App should be granted access to those repositories.


Canonical only uses the App as a fine-grained authorization mechanism.
The SDK doesn't share information with Canonical
or any third party (apart from GitHub).
That said,
if you prefer to use a different authentication mechanism,
export the :envvar:`GITHUB_TOKEN` environment variable inside the workshop.
The :samp:`github-runner` script will use that if available.


Run a workflow locally
----------------------

Now everything is set up to run a workflow locally.


Start the runner
~~~~~~~~~~~~~~~~

Start the Runner client inside the workshop:

.. code-block:: console

   $ workshop exec ci github-runner --label=workshop <OWNER>[/<REPO>]

Replace :samp:`<OWNER>/<REPO>` with the full repository name
(e.g., :samp:`canonical/workshop`).
If omitted,
the script tries to detect this information from the local repository.
For organization-level runners,
make sure to provide the organization name (e.g., :samp:`canonical`).

The :option:`!--label` option adds a label to the runner,
to distinguish it from GitHub-hosted runners
and other self-hosted runners (if any).
Use :option:`!--help` to see the full list of options.

When the script runs for the first time,
it will request authorization
using a one-time code.
Access can be revoked at any time via the `GitHub App
<https://github.com/apps/test-app-jonathan-conder-1>`_.

After a few seconds,
the runner should be ready for incoming jobs.
The next step is to configure jobs to use the runner.


Runner options
~~~~~~~~~~~~~~

The :samp:`github-runner` command supports several options:

.. code-block:: console

   $ workshop exec ci github-runner --help


Key options include:

.. list-table::
  :header-rows: 1

  * - Option
    - Description

  * - :option:`!--name`
    - Specify a unique runner name

  * - :option:`!--prefix`
    - Add a prefix to the runner name (defaults to hostname)

  * - :option:`!--label`
    - Add custom labels to the runner

  * - :option:`!--once`
    - Exit after running a single job

  * - :option:`!--group-id`
    - Add runner to a specific runner group


Configure your workflow
~~~~~~~~~~~~~~~~~~~~~~~

Add the :samp:`workshop` label
to the :samp:`runs-on` option in the workflow file.

Consider making this configurable,
if only to avoid repeatedly editing the workflow.
For example:

.. code-block:: yaml
   :caption: .github/workflows/test.yaml
   :emphasize-lines: 5-12,15

   on:
     pull_request:
     push:
       branches: [main]
     workflow_dispatch:
       inputs:
         runner:
           description: Where to run the job
           type: choice
           required: true
           options: [ubuntu-latest, workshop]
           default: ubuntu-latest
   jobs:
     test:
       runs-on: ["${{ inputs.runner || 'ubuntu-latest' }}"]
       steps:
         - uses: actions/checkout@v6
         - run: make test


Run the workflow
~~~~~~~~~~~~~~~~

The specific steps depend on the workflow.

For the above example:
commit the updated workflow to the :samp:`main` branch,
find it in the :guilabel:`Actions` tab of the repository,
and select :guilabel:`Run workflow`.
Pick whichever branch you like,
as long as the runner is set to :samp:`workshop`.

The Runner client should print a few logs when a job starts and finishes.
Full logs can still be viewed on GitHub.


Tips
----

Take care when logging.
Some actions could leak sensitive information about the runner,
such as its IP address.

----

The Runner client runs one job at a time.
To run several jobs in parallel,
use multiple workshops.
For example:

.. code-block:: console

   $ mkdir -p .workshop
   $ mv workshop.yaml .workshop/ci.yaml
   $ sed 's/name: ci/name: ci2/' <.workshop/ci.yaml >.workshop/ci2.yaml
   $ workshop launch ci2
   $ workshop exec ci2 github-runner --label=workshop

----

The Runner client doesn't clean up after itself.
This can be helpful for debugging
but may cause issues for some workflows.
To avoid these issues,
refresh the workshop after each job.
For example:

.. code-block:: console

   $ while workshop exec ci github-runner --label=workshop --once; do
       workshop refresh ci
     done

----

In rare cases (like a power outage at the wrong time),
runners can remain attached to the repository indefinitely.
These can be removed manually in the repository or organization settings.

----

For quick iteration,
the runner can be made conditional on the branch name:

.. code-block:: yaml
   :caption: .github/workflows/test.yaml
   :emphasize-lines: 5,8

   on:
     push:
       branches:
         - main
         - workshop-runner/**
   jobs:
     test:
       runs-on: ["${{ startsWith(github.ref_name, 'workshop-runner/') && 'workshop' || 'ubuntu-latest' }}"]

       steps:
         - uses: actions/checkout@v6
         - run: make test


.. _how_run_github_actions_locally_security:

Security considerations
-----------------------

When running actions locally:

- Be cautious with secrets and sensitive data
- Mind that actions may leak information about your local environment
- Consider using separate workshops for different projects
- Regularly review and rotate access tokens
- Monitor actions for unexpected behavior


See also
--------

How-to guides:

- :ref:`how_git_workshops`

Tutorial:

- :ref:`tut_sketch_sdks`
