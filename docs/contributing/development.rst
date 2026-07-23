:relatedlinks: [Spread](https://github.com/canonical/spread), [golangci-lint](https://golangci-lint.run/), [gocheck](https://pkg.go.dev/gopkg.in/check.v1#section-readme)

.. _contributing_development:

.. meta::
   :description: Make a code change to Workshop. Set up, develop, test, and
                 review your change within the project's standard workflow.

Contribute to development
=========================

A code change to |ws_markup| moves from a local development environment,
through testing and review, to a merged pull request.


Set up your work environment
----------------------------

|ws_markup| has a client-server architecture.
Its :program:`workshopd` daemon exposes a RESTful API
(see :file:`internal/daemon/api.go`)
to the clients.

|ws_markup| also develops itself in a workshop.
The repository is a :ref:`project <exp_projects>`
whose :file:`.workshop/dev.yaml` definition
describes a workshop named :samp:`dev`.
It carries the Go toolchain and the linters,
pinned to the versions the project checks against,
and packages them as :ref:`actions <exp_workshop_definition_actions>`.

Your work is split across two environments:

.. list-table::
   :header-rows: 1
   :widths: 25 75

   * - Environment
     - What runs there

   * - The :samp:`dev` workshop
     - Go builds, :program:`golangci-lint`, :program:`shellcheck`,
       unit tests and coverage,
       and the documentation build, preview, and checks.

   * - The host
     - :program:`workshopd`, the :program:`spread` suites,
       and the LXD integration tests.

:program:`workshopd` connects to LXD
and expects the ZFS storage driver,
neither of which the :samp:`dev` workshop ships,
so the host-side commands run outside it.


.. _contributing_dev_workshop:

Launch the dev workshop
~~~~~~~~~~~~~~~~~~~~~~~

Install |ws_markup| and LXD first
(see :ref:`tut_install`),
then launch the workshop from the repository root:

.. code-block:: console

   $ workshop launch dev

The definition pins the toolchain
to the versions the project checks against:

- The :samp:`go` SDK tracks :samp:`1.26/stable`,
  matching the :samp:`go` directive in :file:`go.mod`.

- :program:`golangci-lint` is pinned to v2.11.3,
  matching the :samp:`rev` in :file:`.pre-commit-config.yaml`;
  the linting workflow in :file:`.github/workflows/lint.yaml`
  tracks the same v2.11 minor version.

Linting in the workshop therefore uses the same version
as the linting workflow,
without installing the linters on your host.

The tooling comes from an :ref:`in-project SDK <exp_in_project_sdk>`
in :file:`.workshop/tools/`,
which the definition lists as :samp:`project-tools`;
the :samp:`project-` prefix selects the in-project source
(see :ref:`ref_workshop_definition_sdk_entry`).
Its :ref:`hooks <exp_sdk_hooks>` install :program:`make`,
:program:`shellcheck`, :program:`golangci-lint`, and snapd-testing-tools,
and report the workshop unhealthy
when :program:`golangci-lint` is missing
(see :ref:`exp_workshopctl_health`).
The SDK also slots a :ref:`tunnel <exp_tunnel_interface>`
that the :ref:`system SDK <exp_system_sdk>` plugs
to publish the documentation preview
on the host at :samp:`127.0.0.1:8000`
(see :ref:`how_forward_ports`).

List the available actions and run one:

.. code-block:: console

   $ workshop actions dev
   $ workshop run dev lint

Because :samp:`dev` is the only definition in :file:`.workshop/`,
you can omit its name.
See :ref:`ref_workshop_actions` and :ref:`ref_workshop_run`.

Open an interactive shell with :ref:`workshop shell <ref_workshop_shell>`,
run a single command with :ref:`workshop exec <ref_workshop_exec>`,
and apply edits to :file:`.workshop/dev.yaml`
or to the SDK in :file:`.workshop/tools/`
with :ref:`workshop refresh <ref_workshop_refresh>`.


Run the daemon on the host
~~~~~~~~~~~~~~~~~~~~~~~~~~

The recommended way to run the current sources
is the :command:`go tool try` development tool
wired into :file:`go.mod`:

.. code-block:: console

   $ go tool try

This builds :file:`./cmd/...`
into a temporary session directory under :file:`try_sessions/`,
starts :program:`workshopd` against it,
and drops you into a subshell
with :envvar:`WORKSHOP_DATA`, :envvar:`WORKSHOP_CACHE`,
:envvar:`WORKSHOP_SOCKET`, and :envvar:`PATH` pre-configured.
Exit the shell to tear the session down.
Pass :option:`!--keep` to retain the session directory for inspection.
Run :command:`go tool try` again from inside the shell
to rebuild and restart :program:`workshopd` in place.

To run :program:`workshopd` directly:

.. code-block:: console

   $ go install ./cmd/...
   $ export WORKSHOP_DATA=~/workshop
   $ export WORKSHOP_CACHE=~/workshop-cache
   $ export WORKSHOP_DEBUG=1
   $ workshopd run --create-dirs

The client can connect using the daemon's Unix domain socket:

.. code-block:: console

   $ export WORKSHOP_DATA=~/workshop
   $ workshop list


Install Spread
~~~~~~~~~~~~~~

`Spread <https://github.com/dmitry-lyfar/spread>`__ is the end-to-end testing tool
for |ws_markup|.
It launches its own LXD containers,
so install and run it on the host:

.. code-block:: console

   $ git clone https://github.com/canonical/spread
   $ cd spread
   $ go install ./...

Make sure the :file:`$GOPATH/bin/` directory
is included in :envvar:`PATH`.
After successful installation, you should see the help message by running:

.. code-block:: console

   $ spread -h

Run the documentation end-to-end test suites with:

.. code-block:: console

   $ spread tests/docs-tutorial/
   $ spread tests/docs-how-to/



Choose a task
-------------

Work is tracked in the project's
`issue tracker <https://github.com/canonical/workshop/issues>`__.
Browse the open issues and comment on one to signal that you're taking it on.

For a small, self-contained fix, you can open a pull request directly.
For a larger or more involved change, open an issue first
to agree on the approach with the maintainers
before you invest time in the implementation.


Draft your work
---------------

Create a branch
~~~~~~~~~~~~~~~

Name your branch after the type of change and a short description.
|ws_markup| uses the commit type as a branch prefix, for example:

- :samp:`feat/workspace-start`
- :samp:`fix/spread-tests-github`
- :samp:`chore/update-lxd`


Develop
~~~~~~~

Keep each change focused and reversible.
When a decision might be costly to reverse,
state the rationale in the pull request description
so reviewers can follow the reasoning and collaborate on it.

|ws_markup| follows a set of Go conventions for naming, error handling,
error messages, and code structure.
See the :ref:`coding_style_guide` for the full set of patterns and their rationale.


Test
~~~~

Run the checks before submitting a pull request.
The :samp:`dev` workshop runs them
with the tool versions the project pins
(see :ref:`contributing_dev_workshop`);
each check also has a host-native equivalent
if you have the tools installed.

|ws_markup| tests use
`gocheck <https://pkg.go.dev/gopkg.in/check.v1#section-readme>`_,
which integrates with :command:`go test`.
Run the unit tests with coverage:

.. code-block:: console

   $ workshop run dev cover

Or on the host:

.. code-block:: console

   $ go test ./...
   $ go test -covermode=count -coverpkg=./... -coverprofile=coverage.out ./...

To run a single test or suite, name the package first:

.. code-block:: console

   $ workshop exec dev -- go test <PACKAGE> -check.f <TEST-NAME|SUITE-NAME>

To control the coverage scope and mode:

.. code-block:: console

   $ workshop exec dev -- go test -coverpkg=<./...|PACKAGE> \
       -covermode=<set|count|atomic> \
       -coverprofile=<OUTPUT-FILE> <./...|PACKAGE>

The :samp:`cover` action writes its profile to :file:`coverage.out`.
Because the project directory is mounted into the workshop at :file:`/project/`,
you can render the profile on the host:

.. code-block:: console

   $ go tool cover -html=<OUTPUT-FILE> -o <OUTPUT-HTML>

The output flag can be omitted to open in the default browser:

.. code-block:: console

   $ go tool cover -html=coverage.out

Formatting and common pitfalls are checked with
`golangci-lint <https://golangci-lint.run/>`_.
Lint the sources in both the full and the incremental configuration:

.. code-block:: console

   $ workshop run dev lint

Or on the host:

.. code-block:: console

   $ golangci-lint run
   $ golangci-lint run --new-from-rev='HEAD~' --config=.golangci.incremental.yaml

Some issues can be fixed automatically:

.. code-block:: console

   $ workshop exec dev -- golangci-lint run --fix

Or on the host:

.. code-block:: console

   $ golangci-lint run --fix

Check the shell scripts and the :program:`spread` task definitions:

.. code-block:: console

   $ workshop run dev shellcheck

If `pre-commit <https://pre-commit.com/index.html#install>`_ is available,
:program:`git` can run the linters on every commit
using the same pinned :program:`golangci-lint`:

.. code-block:: console

   $ pre-commit install

The LXD integration tests sit behind a build tag
and drive LXD directly, so run them on the host:

.. code-block:: console

   $ go test -tags=integration ./internal/workshop/lxd/tests/integration/

Run the end-to-end suites with
`Spread <https://github.com/canonical/spread>`__, also on the host:

.. code-block:: console

   $ spread tests/<TEST-PATH-NAME>

Integration tests run through :program:`spread`
create the coverage profile automatically,
but the artifacts need to be collected from the VM
with the :option:`!-artifacts` flag:

.. code-block:: console

   $ spread -artifacts=<PATH-TO-DEST> tests/integration/


Document your work
~~~~~~~~~~~~~~~~~~

Update the documentation to match your change:
new behavior, changed flags, or removed features all belong in the docs.
See :ref:`contributing_documentation` for how to write, build, and test them.


Commit
~~~~~~

|ws_markup| commit messages differ
from conventional commits in capitalization:

.. code-block:: none

   Ensure correct permissions and ownership for the content mounts

    * Work around an LXD issue regarding empty dirs:
      https://github.com/canonical/lxd/issues/12648

    * Ensure the source directory is owned by the user running a workshop.

   Links:
   - ...
   - ...

The messages rarely, if ever, state the type of the commit
(:samp:`fix`, :samp:`feat`, and so on);
these are used for branch naming instead.


Review with the team
--------------------

Send for review
~~~~~~~~~~~~~~~

Open a `pull request <https://github.com/canonical/workshop/pulls>`_
against the repository.
Fill in the pull request template checklist,
which largely reiterates the expectations covered here.


Address quality concerns
~~~~~~~~~~~~~~~~~~~~~~~~

Your pull request runs a set of automatic checks for linting, unit tests,
end-to-end tests, and security scanning.
For the full catalog of workflows, see :ref:`contributing_cicd`.

After receiving review comments, optimize for commit history clarity.
Address them with
`fixup commits <https://git-scm.com/docs/git-commit/2.32.0#Documentation/git-commit.txt---fixupamendrewordltcommitgt>`_
and rebase using
`autosquash <https://git-scm.com/docs/git-rebase#Documentation/git-rebase.txt---autosquash>`_
when reasonable.


Wrap up the review
~~~~~~~~~~~~~~~~~~

Once the checks pass and a maintainer approves the change,
it's merged into the target branch.
Keep an eye on the pull request in case follow-up questions come up.


Get help and support
--------------------

If you get stuck, ask on the issue you're working on,
or open a new one in the
`issue tracker <https://github.com/canonical/workshop/issues>`__.
