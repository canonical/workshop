.. _contributing_maintenance:

.. meta::
   :description: Maintainer guide for Workshop, combining release procedures
                 with reference information about documentation, continuous
                 integration, and repository tooling.

Maintain the project
====================

This maintainer guide combines
task-oriented release and documentation procedures
with reference information about automation and repository tooling.
Use it for work that sits outside the day-to-day contribution flow.


Releases
--------

See the :ref:`release notes <release_notes>`
for more information on the general approach.
The steps to produce a |ws_markup| release are as follows.


Build the snaps locally
~~~~~~~~~~~~~~~~~~~~~~~

`Snapcraft <https://documentation.ubuntu.com/snapcraft/stable/>`_
is used to build, package, and publish :program:`workshop` snaps.
All these processes run in a self-launched
`LXD <https://canonical.com/lxd/docs/latest/>`_ container.
To run the build,
install :program:`snapcraft` and :program:`lxd` using :program:`snap`:

.. code-block:: console

   $ sudo snap install --classic snapcraft
   $ sudo snap install --channel=6/stable lxd

Add the current user to the :samp:`lxd` group
to give permission to access its resources:

.. code-block:: console

   $ sudo usermod -a -G lxd $USER

Log out and reopen your user session for the new group to become active,
then initialize LXD:

.. code-block:: console

   $ lxd init


Publish the release
~~~~~~~~~~~~~~~~~~~

Here's the publishing checklist to follow:

- Merge and close the outstanding pull requests from the release scope

- Make sure the unit, integration, and documentation tests are green;
  see :ref:`contributing_development` and :ref:`contributing_documentation` for details

- Update the documentation;
  see :ref:`contributing_doc_release` for the full checklist

- Create and push a new release tag with :program:`git`,
  using `semantic versioning <https://semver.org/>`_

- Run the `release workflow
  <https://github.com/canonical/workshop/actions/workflows/release.yaml>`_
  on GitHub;
  this builds and publishes release snaps
  for the supported architectures,
  creates a GitHub release,
  and adds a pull request to update the
  :ref:`CLI reference <ref_workshop__cli>`

- Generate the
  `change log <https://github.com/canonical/workshop/releases/new>`_
  on GitHub


.. _contributing_doc_release:

Release documentation
---------------------

At every release, remember to:

- Merge the auto-generated CLI reference pull request.

- Bump the snap revision used across the docs.

- Refresh the three schema files
  under :file:`docs/reference/definition-files/`.

  Regenerate :file:`schema-sdk.json` and :file:`schema-sdkcraft.json`
  in a local |sdk_markup| repository checkout
  and copy the outputs over:

  .. code-block:: console

     $ cd <PATH-TO-SDKCRAFT-CHECKOUT>
     $ uv run python sdkcraft/models/metadata.py
     $ uv run python sdkcraft/models/project.py
     $ cp schema-sdk.json schema-sdkcraft.json \
          <PATH-TO-WORKSHOP-CHECKOUT>/docs/reference/definition-files/

  The |ws_markup| :file:`schema.json` is hand-audited
  against :file:`internal/workshop/workshop_file.go`.

- Update the `release notes <https://github.com/canonical/workshop/releases>`_
  with relevant details, following the established format;
  for an |sdk_markup| release,
  update the respective section in the same manner.

  |ws_markup| and |sdk_markup| are versioned independently,
  so name the |sdk_markup| version in its own heading.
  When |sdk_markup| did not release alongside |ws_markup|,
  carry its last released version forward,
  mark the heading :samp:`### SDKcraft <VERSION> (unchanged)`,
  and omit the |sdk_markup| line from the changelog footer.

- Copy the release notes
  to the documentation under :file:`docs/release-notes/`
  and update the latest version and the version table
  in :file:`docs/release-notes/index.rst`;
  the recent version lists should contain versions from the last 6 months.

- Refresh the
  `coverage map <https://github.com/canonical/workshop/blob/main/docs/coverage.md>`_
  by running the :file:`.github/workflows/doc-cover.yaml` workflow
  and merging the resulting pull request.

- Copy the auto-generated |sdk_markup| CLI reference
  from the `SDKcraft repository <https://github.com/canonical/sdkcraft>`__
  to :file:`docs/reference/cli/` as :file:`sdkcraft*.rst`,
  making sure the updated documentation builds properly.


.. _contributing_doc_generation:

CLI reference generation
------------------------

The :ref:`command-line reference <ref_workshop__cli>` for |ws_markup|
is produced directly from the Cobra command tree:

.. code-block:: console

   $ go run ./cmd/workshop generate-docs

The helper in :file:`cmd/workshop/gendocs.go`
uses the `Gencodo <https://github.com/canonical/gencodo>`_ Go module
to convert the command metadata into :file:`*.rst` files with templates.
In particular, this is used during the
:ref:`release workflow <contributing_cicd>`.

The :ref:`command-line reference <ref_sdkcraft__cli>` for |sdk_markup|
can be generated in the |sdk_markup| repository.
Run :file:`gendocs.py` there to generate the files.
The current implementation relies on
`craft-application <https://github.com/canonical/craft-application/>`__
and doesn't fully integrate with |ws_markup| documentation yet.


.. _contributing_cicd:

CI/CD
-----

Multiple
`GitHub Actions
<https://docs.github.com/en/actions/get-started/understand-github-actions>`_
workflows,
defined in the :file:`.github/workflows/` directory,
automate testing, building, documentation, and release processes.

Some of these workflows come from the
:ref:`starter pack <contributing_doc_structure>` (marked SP),
while others are custom-made for |ws_markup|'s needs.

Documentation workflows:

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Workflow
     - Purpose

   * - :file:`automatic-doc-checks.yml` (SP)
     - Build the documentation and fail on Sphinx warnings.

   * - :file:`doc-cover.yaml`
     - Generate and update the documentation coverage map.

   * - :file:`doc-update-sdk-schema.yml`
     - Update SDK schema files from the |sdk_markup| repository.

   * - :file:`fix-redirected-links.yml`
     - Update selected redirecting documentation links and open a pull request.

   * - :file:`markdown-style-checks.yml` (SP)
     - Lint Markdown documentation files.

   * - :file:`update-sphinx-stack.yaml`
     - Update Sphinx Stack files and documentation dependencies weekly
       and on demand.


Code quality and testing workflows:

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Workflow
     - Purpose

   * - :file:`cover.yaml`
     - Orchestrates :file:`spread.yaml` and :file:`unit-tests.yaml`;
       aggregates coverage reports.

   * - :file:`fixup.yaml`
     - Check for fixup and squash commits in pull requests.

   * - :file:`lint.yaml`
     - Run :program:`golangci-lint` on Go code.

   * - :file:`scanning.yml`
     - Scan for known security vulnerabilities using Trivy.

   * - :file:`spread.yaml`
     - Run end-to-end tests with Spread (reusable workflow).

   * - :file:`staging.yaml`
     - Prevent staged test SDKs from being merged.

   * - :file:`unit-tests.yaml`
     - Run Go unit tests and check for race conditions (reusable workflow).

   * - :file:`zizmor.yaml`
     - Audit GitHub Actions workflows for security issues.


Build and release workflows:

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Workflow
     - Purpose

   * - :file:`build-deps.yaml`
     - Build and cache the |ws_markup| snap (reusable workflow).

   * - :file:`lxd-candidate-check.yaml`
     - Test |ws_markup| against the LXD candidate channel daily;
       uses :file:`build-deps.yaml`.

   * - :file:`release.yaml`
     - Build release snaps for ARM64 and X64;
       create GitHub release and trigger CLI docs update PR.


.. _contributing_copilot:

Copilot configuration
---------------------

The repository includes configurations
to help GitHub Copilot provide assistance;
these are located in the :file:`.github/` directory
and include general instructions
as well as custom agents and reusable skills for specific tasks.


Copilot instructions
~~~~~~~~~~~~~~~~~~~~

The :file:`.github/copilot-instructions.md` file
provides general project context to GitHub Copilot.

Also, there are documentation- and code-specific instructions
in :file:`.github/docs.instructions.md`
and :file:`.github/go.instructions.md`,
tailored to guide Copilot when assisting with documentation and Go code tasks,
respectively.


Agents and skills
~~~~~~~~~~~~~~~~~

The :file:`.github/agents/` subdirectory contains
`custom agent prompts
<https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/create-custom-agents>`__
for review and maintenance tasks:

- :file:`code-review.agent.md`:
  A code review specialist that enforces commit message standards,
  coding conventions, and error handling patterns,
  referencing the :ref:`development contribution guide <contributing_development>`
  and the :ref:`coding style guide <coding_style_guide>`.

- :file:`doc-schema-update.agent.md`:
  A specialized agent for reconciling
  the JSON schema in :file:`docs/reference/definition-files/schema.json`
  with the validation logic
  in :file:`internal/workshop/workshop_file.go`.

The :file:`.github/skills/` subdirectory contains reusable skills.
The :file:`.github/skills/documentation-review/SKILL.md` skill
orchestrates build validation,
content analysis, accuracy verification, structure checks, and style review
using the :ref:`documentation contribution guide <contributing_documentation>`
and the :ref:`documentation style guide <doc_style_guide>`.

These agents and skills provide structured, actionable feedback
and help maintain consistency across contributions.
