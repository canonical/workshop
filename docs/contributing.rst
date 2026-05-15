.. _contributing:

.. meta::
   :description: Guide on contributing to the Workshop project, detailing
                 why and how to join the community, including instructions for
                 code contributions, documentation improvements, releases,
                 and testing opportunities.

How to contribute
=================

We believe everyone has something valuable to contribute,
whether you're a coder, a writer, or a tester.
Here's how and why you could get involved:

- **Why join us**:
  Work with like-minded people, grow your skills,
  connect with diverse professionals, and make a difference.

- **What do you get**:
  Personal growth, recognition for your contributions,
  early access to new features, and the joy of seeing your work appreciated.

- **Start early, start simple**:
  Dive into code contributions,
  improve documentation, or be among the first testers.
  Your presence matters, regardless of experience or the scale of your input.

The guidelines below will keep your contributions effective and meaningful.



Environment setup
-----------------
#. ``Workshop`` has a client-server architecture.
   Its ``workshopd`` daemon exposes a RESTful API (see ``internal/daemon/api.go``) to the clients.
   To run the daemon locally:

   .. code-block:: console

      go install ./...
      export WORKSHOP=~/workshop
      export WORKSHOP_DEBUG=1
      workshopd run --create-dirs

   The client can connect using the daemon's Unix domain socket:

   .. code-block:: console

      export WORKSHOP=~/workshop
      workshop list

#. ``Spread`` is the end-to-end testing tool for ``workshop``.
   Install it from `GitHub <https://github.com/canonical/spread>`__:

   .. code-block:: console

      git clone https://github.com/canonical/spread
      cd spread
      go install ./...


   Make sure the ``$GOPATH/bin`` directory is included in ``$PATH``.
   After successful installation, you should see the help message by running:

   .. code-block:: console

      spread -h

   To run the end-to-end test suite ``tests/documentation/``,
   download the latest SDKcraft release from the `repository <https://github.com/canonical/sdkcraft/releases>`_
   and move it to the ``tests/`` directory.

Coding
------

In Workshop, commit messages differ from conventional commits in capitalization:

.. code-block:: none

   Ensure correct permissions and ownership for the content mounts

    * Work around an LXD issue regarding empty dirs:
      https://github.com/canonical/lxd/issues/12648

    * Ensure the source directory is owned by the user running a workshop.

   Links:
   - ...
   - ...


The messages rarely, if ever, state the type of the commit
(e.g., ``fix``, ``feat``, etc.);
these are used for branch naming, for example:

- ``feat/workspace-start``
- ``fix/spread-tests-github``
- ``chore/update-lxd``


Commits that focus on docs must use the ``Doc:`` type prefix
with an optional scope in square brackets:

.. code-block:: none

   Doc[chore]: Align references


PR descriptions should follow the PR template checklist,
which largely reiterates this section.


After receiving review comments,
optimize for commit history clarity.
Address review comments with 
`fixup commits <https://git-scm.com/docs/git-commit/2.32.0#Documentation/git-commit.txt---fixupamendrewordltcommitgt>`_ 
and rebase using 
`autosquash <https://git-scm.com/docs/git-rebase#Documentation/git-rebase.txt---autosquash>`_ 
when reasonable.


Reversibility
~~~~~~~~~~~~~

When making decisions that might be costly to reverse,
explicitly state the rationale in the PR description.
This helps to understand the reasoning and collaborate better.


Coding standards
~~~~~~~~~~~~~~~~

- **Avoid nested conditions**:
  Refrain from nesting conditions to enhance readability and maintainability.

- **Eliminate dead code and redundant comments**:
  Remove unused or obsolete code and comments.
  This promotes a cleaner code base and reduces confusion.

- **Normalize symmetries**:
  Handle identical operations consistently, using a uniform approach.
  This also improves consistency and readability.


Error handling
~~~~~~~~~~~~~~

When handling errors or multiple returns,
follow a consistent pattern:

.. code-block:: go

   // one way to handle errors
   if err := f(); err != nil {
      ...
   }

   // one way to handle multiple returns
   val, err := f()
   if err != nil {
      ...
   }


Error messages
~~~~~~~~~~~~~~

- **Be consistent**:
  Try to match the style of existing error messages.
  Most of these can be found by searching for ``fmt.Errorf`` and ``errors.New``.
  Paths and other identifiers should be double-quoted if possible.

- **Quote values consistently**:
  Use ``%q`` for interpolated identifiers
  and ``"..."`` inside backtick raw strings for literal flag and command references.
  See :ref:`coding_style_guide` for details.

- **Consider the user experience**:
  Error messages should be clear and actionable.

- **Be specific**:
  For example, if a file was not found, the error message should include its path.

- **Mind the nesting**:
  Start in lowercase and avoid trailing punctuation.
  Avoid excessively long or repetitive error chains.
  A common template is: ``what was attempted: why it went wrong``.


Code structure
~~~~~~~~~~~~~~

- **Check coupled code elements**:
  Verify that coupled code elements, files and directories are adjacent.
  For instance, store test data close to the corresponding test code.

- **Group variable declaration and initialization**:
  Declare and initialize variables together
  to improve code organization and readability.

- **Divide large expressions**:
  Break down large expressions
  into smaller self-explanatory parts.
  Use multiple variables if necessary
  to make the code more understandable
  and choose names to reflect their purpose.

- **Use blank lines for logical separation**:
  Insert a blank line between two logically distinct sections of code.
  This improves its structure and makes it easier to comprehend.


Linting
-------

Code should be formatted consistently
and avoid common pitfalls.
Contributions will be checked for some of these issues
using `golangci-lint <https://golangci-lint.run/>`_.
To run these checks locally:

.. code-block:: console

   golangci-lint run


Some issues can be fixed automatically:

.. code-block:: console

   golangci-lint run --fix


If `pre-commit <https://pre-commit.com/index.html#install>`_ is available,
``git`` can run these checks on every commit:

.. code-block:: console

   pre-commit install


Testing
-------

Make sure to run unit and integration tests before submitting a PR.
We use a ``go test``-compatible
`gocheck <https://pkg.go.dev/gopkg.in/check.v1#section-readme>`_:

.. code-block:: console

   go test ./...
   go test -check.f <TestName|SuiteName>


To run end-to-end tests and integration tests with
`Spread <https://github.com/canonical/spread>`__:

.. code-block:: console

   spread tests/<TestPathName>


To check code coverage:

.. code-block:: console

   go test -coverpkg=<./...|package> -covermode=<set|count|atomic> -coverprofile=<OutputFile> <./...|package>


For example, to measure coverage using all tests:

.. code-block:: console

   go test -covermode=count -coverpkg=./... -coverprofile=cover.out ./...

To generate an HTML representation:

.. code-block:: console

   go tool cover -html=<OutputFile> -o <OutputHTML>


For example:

.. code-block:: console

   go tool cover -html=cover.out -o cover.html


The output flag can be omitted to open in the default browser:

.. code-block:: console

   go tool cover -html=cover.out


The above will work for unit and integration tests instrumented directly with
`go test`. Integration tests run using `spread` will create the coverprofile
automatically, however the artifacts will need to be collected from the VM.
This can be accomplished by using the `-artifacts` flag when running `spread`.

.. code-block:: console

   spread -artifacts=<path-to-dest> tests/integration/


Releases
--------

See the :ref:`release notes <release_notes>`
for more information on our general approach.
The steps to produce a Workshop release are as follows.


Build the snaps locally
~~~~~~~~~~~~~~~~~~~~~~~

`Snapcraft <https://documentation.ubuntu.com/snapcraft/stable/>`_
is used to build, package, and publish ``workshop`` snaps.
All these processes run in a self-launched
`LXD <https://documentation.ubuntu.com/lxd/latest/>`_ container.
To be able to run the build, install ``snapcraft`` and ``lxd`` using ``snap``:

.. code-block:: console

   sudo snap install --classic snapcraft
   sudo snap install --channel=6/stable lxd


Add the current user to the ``lxd`` group
to give permission to access its resources:

.. code-block:: console

   sudo usermod -a -G lxd $USER


Log out and reopen your user session for the new group to become active,
then initialize LXD:

.. code-block:: console

   lxd init


Publish the release
~~~~~~~~~~~~~~~~~~~

Here's the publishing checklist to follow:

- Merge and close the outstanding pull requests from the release scope

- Make sure the unit, integration, and documentation tests are green;
  see `Testing`_ for details

- Update the documentation;
  see the `Release documentation`_ section for the full checklist

- Create and push a new release tag with ``git``,
  using `semantic versioning <https://semver.org/>`_

- Run the `release workflow
  <https://github.com/canonical/workshop/actions/workflows/release.yaml>`_
  on GitHub;
  this builds the release snaps for the supported architectures
  to be published on GitHub
  and adds a pull request to update the
  :ref:`CLI reference <ref_workshop__cli>`

- Generate the
  `change log <https://github.com/canonical/workshop/releases/new>`_
  on GitHub


.. _contributing_copilot:

Copilot configuration
---------------------

The repository includes configurations
to help GitHub Copilot provide assistance;
these are located in the ``.github/`` directory
and include general instructions
as well as customized agent prompts for specific tasks.


Copilot instructions
~~~~~~~~~~~~~~~~~~~~

The ``.github/copilot-instructions.md`` file
provides general project context to GitHub Copilot.

Also, there are documentation- and code-specific instructions
in ``.github/docs.instructions.md`` and ``.github/go.instructions.md``,
tailored to guide Copilot when assisting with documentation and Go code tasks,
respectively.


Custom agents
~~~~~~~~~~~~~

The ``.github/agents/`` subdirectory contains
`custom agent prompts
<https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/create-custom-agents>`__
for specific review and maintenance tasks:

- ``code-review.agent.md``:
  A code review specialist that enforces commit message standards,
  coding conventions, and error handling patterns,
  referencing this contribution guide
  and the :ref:`coding style guide <coding_style_guide>`.

- ``doc-review.agent.md``:
  A technical documentation reviewer
  that performs a multistage review process
  including build validation, content analysis, and style checking,
  referencing this contribution guide
  and the :ref:`documentation style guide <doc_style_guide>`.

- ``doc-schema-update.agent.md``:
  A specialized agent for reconciling
  the JSON schema in ``docs/reference/definition-files/schema.json``
  with the validation logic in ``internal/workshop/workshop_file.go``.


These agents provide structured, actionable feedback
and help maintain consistency across contributions.


.. _contributing_doc:

Documentation
-------------

All documentation resides in the ``docs/`` directory.
To build and run it at ``127.0.0.1:8000``:

.. code-block:: console

   workshop launch
   workshop run docs-run


To suggest changes,
submit a `pull request <https://github.com/canonical/workshop/pulls>`_,
limiting it to the ``docs/`` directory
and prefixing the title with ``Doc:``.


.. _contributing_doc_structure:

Structure and style
~~~~~~~~~~~~~~~~~~~

We use the `Canonical documentation starter pack
<https://github.com/canonical/sphinx-stack>`_
together with a custom Workshop in-project SDK in ``.workshop/``
to run and build our documentation;
our preferred markup is reStructuredText (reST),
with some opinionated style choices evident in the source.

See the relevant documentation before making changes:

- :doc:`Workshop documentation style guide <doc-style-guide>`
  (project-specific conventions and patterns)

- `Starter pack
  <https://documentation.ubuntu.com/sphinx-stack/latest/>`_

- `reST style guide
  <https://documentation.ubuntu.com/sphinx-stack/latest/reference/style-guide/>`_

- `reST cheat sheet
  <https://documentation.ubuntu.com/sphinx-stack/latest/reference/doc-cheat-sheet/>`_


.. _contributing_doc_dependencies:

Dependency management
~~~~~~~~~~~~~~~~~~~~~

The documentation build requires Python 3.11 or later.

Documentation dependencies are managed using ``uv``:

- ``docs/requirements.in`` contains dependencies specific to Workshop docs
- ``docs/requirements.txt`` is the final, resolved dependency file


The final file is generated by the ``update-starter-pack`` workflow,
listed in :ref:`contributing_cicd`.


.. _contributing_doc_generation:

CLI reference
~~~~~~~~~~~~~

The :ref:`command-line reference <ref_workshop__cli>` for Workshop
is produced directly from the Cobra command tree:

.. code-block:: console

   go run ./cmd/workshop generate-docs


The helper in ``cmd/workshop/gendocs.go``
uses the `Gencodo <https://github.com/canonical/gencodo>`_ Go module
to convert the command metadata into ``.rst`` files with clever templates.

In particular, this is used during the
:ref:`release workflow <contributing_cicd>`.

---

The :ref:`command-line reference <ref_sdkcraft__cli>` for SDKcraft
can be generated in the SDKcraft repository;
run ``gendocs.py`` there to generate the files.
Current implementation relies on
`craft-application <https://github.com/canonical/craft-application/>`__
and doesn't fully integrate with Workshop documentation yet.


.. _contributing_doc_release:

Release documentation
~~~~~~~~~~~~~~~~~~~~~

At every release, remember to:

- Merge the auto-generated CLI reference pull request.

- Bump the snap revision used across the docs.

- Refresh the three schema files under ``docs/reference/definition-files/``.

  Regenerate ``schema-sdk.json`` and ``schema-sdkcraft.json`` in a local SDKcraft repository checkout and copy the outputs over:

  .. code-block:: console

     $ cd <PATH_TO_SDKCRAFT_CHECKOUT>
     $ uv run python sdkcraft/models/metadata.py
     $ uv run python sdkcraft/models/project.py
     $ cp schema-sdk.json schema-sdkcraft.json \
          <PATH_TO_WORKSHOP_CHECKOUT>/docs/reference/definition-files/

  The workshop ``schema.json`` is hand-audited against
  ``internal/workshop/workshop_file.go``.


- Update the `release notes <https://github.com/canonical/workshop/releases>`_
  with relevant details, following the established format;
  for an SDKcraft release, update the respective section in the same manner.

- Copy the release notes to the documentation under ``docs/release-notes/``
  and update the latest version in ``docs/release-notes/index.rst``;
  the recent version lists should contain versions from the last 6 months.

- Refresh the
  `coverage map <https://github.com/canonical/workshop/blob/main/docs/coverage.md>`_
  by running the ``.github/workflows/doc-cover.yaml`` workflow
  and merging the resulting pull request.

- Copy the auto-generated SDKcraft CLI reference
  from the `SDKcraft repository <https://github.com/canonical/sdkcraft>`__
  to ``docs/reference/cli/sdkcraft/``,
  making sure the updated documentation builds properly.


.. _contributing_cicd:

CI/CD
-----

Multiple
`GitHub Actions
<https://docs.github.com/en/actions/get-started/understand-github-actions>`_
workflows,
defined in the ``.github/workflows/`` directory,
automate testing, building, documentation, and release processes.

Some of these workflows come from the
:ref:`starter pack <contributing_doc_structure>` (marked SP),
while others are custom-made for Workshop's needs.


Documentation workflows:

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Workflow
     - Purpose

   * - ``automatic-doc-checks.yml`` (SP)
     - Build the documentation and fail on Sphinx warnings.

   * - ``doc-cover.yaml``
     - Generate and update the documentation coverage map.

   * - ``doc-update-sdk-schema.yml``
     - Update SDK schema files from the SDKcraft repository.

   * - ``markdown-style-checks.yml`` (SP)
     - Check style, spelling, and links in Markdown documentation files.

   * - ``sphinx-python-dependency-build-checks.yml`` (SP)
     - Ensure the Sphinx virtual environment can be built from source.

   * - ``update-starter-pack.yaml``
     - Update documentation starter pack files weekly and on demand.


Code quality and testing workflows:

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Workflow
     - Purpose

   * - ``cover.yaml``
     - Orchestrates ``spread.yaml`` and ``unit-tests.yaml``;
       aggregates coverage reports.

   * - ``fixup.yaml``
     - Check for fixup and squash commits in pull requests.

   * - ``lint.yaml``
     - Run ``golangci-lint`` on Go code.

   * - ``scanning.yml``
     - Scan for known security vulnerabilities using Trivy.

   * - ``spread.yaml``
     - Run end-to-end tests with Spread (reusable workflow).

   * - ``unit-tests.yaml``
     - Run Go unit tests and check for race conditions (reusable workflow).


Build and release workflows:

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Workflow
     - Purpose

   * - ``build-deps.yaml``
     - Build and cache Workshop snap (reusable workflow).

   * - ``lxd-candidate-check.yaml``
     - Test Workshop against LXD candidate channel daily;
       uses ``build-deps.yaml``.

   * - ``release.yaml``
     - Build release snaps for ARM64 and X64;
       create GitHub release and trigger CLI docs update PR.
