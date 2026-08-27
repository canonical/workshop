.. _how_build_sdk:

.. meta::
   :description: How-to guide for building a Workshop SDK with SDKcraft, covering
                 scaffolding from the canonical/template-sdk repository, defining
                 parts and interfaces, authoring the hooks, and iterating on
                 the SDK locally.

How to build an SDK
===================

.. @tests in tests/docs-how-to/build-an-sdk/task.yaml

.. @artefact SDK
.. @artefact SDK definition
.. @artefact SDK hook
.. @artefact SDK part
.. @artefact sdkcraft (CLI)
.. @artefact setup-base
.. @artefact setup-project
.. @artefact check-health
.. @artefact save-state
.. @artefact restore-state
.. @artefact try SDK

A standalone SDK is a |sdk_markup| project
with its own :file:`sdkcraft.yaml`,
its own :file:`hooks/` tree,
and the parts and interfaces
that describe what it ships
and how it integrates with workshops.
Build one by laying out the project,
declaring parts and interfaces,
authoring the lifecycle hooks,
and exercising the result locally
before any thought of publishing.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- |sdk_markup| installed.

- LXD 6.8 or later running on the host.

- |ws_markup| installed and configured
  so that you can launch a workshop
  to try the SDK against.


.. _how_build_sdk_scaffold:

Start from the template
-----------------------

New SDKs start from
`canonical/template-sdk <https://github.com/canonical/template-sdk>`_,
a GitHub-template repository
that ships the project skeleton:

- a :file:`sdkcraft.yaml` ready to be filled in,

- a :file:`hooks/` tree with stubs for the lifecycle hooks,

- a :file:`VERSION` file pinning the upstream release the SDK wraps,

- a :file:`renovate.json` that tracks upstream releases on a long-lived
  version branch and opens PRs to bump :file:`VERSION` as they ship,

- CI workflows under :file:`.github/workflows/` that build on pull requests
  and upload to the SDK Store on push to the version branch
  (the upload workflow needs SDK Store credentials,
  configured once as a GitHub Actions secret;
  see :ref:`how_publish_sdk_ci`),

- a README template aligned to the rest of the project shape.


Use it via GitHub's "Use this template" button,
or :command:`git clone` if you don't host on GitHub.
The choice that follows is how to fill the template in.


With the `design-sdk` skill
~~~~~~~~~~~~~~~~~~~~~~~~~~~

`use-workshop-skill <https://github.com/canonical/use-workshop-skill>`_
ships an agentic skill named :samp:`design-sdk`.
The skill runs an interactive design conversation:
it asks about the software to package,
how upstream distributes it,
what must persist across refreshes,
which network services and hardware it needs,
and which bases and architectures to build for,
then proposes a design
and writes the corresponding files into the template
once you approve it.

#. Install the skill for your agent;
   see :ref:`ref_ai_design_sdk_skill`.

#. Aim the agent at the new repository.

#. Run :samp:`/design-sdk new <SOFTWARE>` and answer the prompts.

#. Approve the proposed design,
   then review the generated files
   and adjust where the skill's defaults don't match your case.


By editing the template directly
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Without the agentic skill,
edit the template files in place.
Fill :file:`sdkcraft.yaml` per :ref:`how_build_sdk_metadata`,
replace each hook stub under :file:`hooks/` per :ref:`how_build_sdk_hooks`,
update :file:`VERSION` to the upstream release you intend to ship first,
and adjust :file:`renovate.json` if the upstream project lives somewhere
other than the GitHub release page the default config targets.


.. _how_build_sdk_metadata:

Fill in the metadata
--------------------

|sdk_markup| needs four pieces of metadata to identify and build the SDK:
:samp:`name`, :samp:`version`, a one-line :samp:`summary`,
and the :samp:`platforms` to build for.
Add :samp:`license` to declare the SDK's licensing terms,
and :samp:`description` for a multi-line write-up:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   name: <NAME>
   version: "<VERSION>"
   summary: One-line description of the SDK
   description: |
     A longer description that explains
     what the SDK packages
     and any noteworthy behavior.
   license: MIT
   platforms:
     ubuntu@22.04:amd64:
     ubuntu@24.04:amd64:


Use the SDK's upstream version for :samp:`version`
when the SDK wraps a single tool;
keep it quoted so that values like :samp:`1.0` aren't parsed as floats.

|sdk_markup| builds one artifact per entry in :samp:`platforms`.
Each entry pairs an Ubuntu base
with a CPU architecture from the Debian naming scheme
(:samp:`amd64`, :samp:`arm64`, and so on).
For SDKs that don't ship compiled binaries,
use :samp:`all` instead of a specific architecture.


Define parts
------------

:ref:`Parts <exp_sdk_parts>` describe how |sdk_markup|
obtains the SDK's payload at build time.
A small SDK often gets by with a single part;
larger SDKs split work along functional boundaries.

For a binary downloaded from a release page,
use the :samp:`dump` plugin with a tarball source:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   parts:
     <NAME>:
       plugin: dump
       source: https://example.com/releases/v${CRAFT_PROJECT_VERSION}/<NAME>-linux-${CRAFT_ARCH_BUILD_FOR}.tar.gz
       source-type: tar


:samp:`${CRAFT_PROJECT_VERSION}` and :samp:`${CRAFT_ARCH_BUILD_FOR}`
expand at build time
from the :samp:`version` field
and the platform |sdk_markup| is currently building for.

If the SDK ships supporting files,
add them as separate parts with the :samp:`file` source type:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   parts:
     <NAME>:
       plugin: dump
       source: https://example.com/releases/v${CRAFT_PROJECT_VERSION}/<NAME>-linux-${CRAFT_ARCH_BUILD_FOR}.tar.gz
       source-type: tar
     service-unit:
       plugin: dump
       source: <NAME>.service
       source-type: file


For source-built SDKs,
the :samp:`rust`, :samp:`go`, and :samp:`python` plugins
take over from :samp:`dump`.
The Craft Parts
`plugin reference <https://documentation.ubuntu.com/craft-parts/latest/reference/plugins/>`_
lists every plugin and its options.


Declare plugs and slots
-----------------------

Plugs and slots wire the SDK
to host resources and to other SDKs in the workshop.
For instance,
an SDK may reach for a :samp:`mount` plug for cache or model directories
that should survive :command:`workshop refresh`,
a :samp:`gpu` plug for GPU acceleration,
or a :samp:`tunnel` slot for services
that expose a network endpoint.

For the mount and tunnel declarations step by step,
including the required attributes
and how |ws_markup| connects each one,
see :ref:`how_declare_plugs_slots`.


.. _how_build_sdk_hooks:

Author the hooks
----------------

Hooks are the run-time logic of an SDK.
|ws_markup| runs them at specific lifecycle stages
of the workshop they're installed in.
All hooks are shell scripts in :file:`hooks/<HOOK-NAME>`
and are linted with `ShellCheck <https://www.shellcheck.net/>`_
when the SDK is packed.

:envvar:`SDK` is available inside every hook.
It points to the SDK's installation root inside the workshop;
use it to reference files the SDK ships,
for example :samp:`"$SDK/bin/<NAME>"`.

:envvar:`SDK_STATE_DIR` is available only inside
:samp:`save-state` and :samp:`restore-state`.
It points to a temporary directory
|ws_markup| creates for one :command:`workshop refresh` cycle:
:samp:`save-state` writes to it before the old workshop is destroyed,
and :samp:`restore-state` reads from it
once the new workshop is up.
The directory is gone as soon as the workshop stops,
so don't use it for long-lived data;
back that with a :samp:`mount` plug or store it in the project directory instead.

|ws_markup| recognizes five hook names:
:samp:`setup-base`, :samp:`setup-project`, :samp:`check-health`,
:samp:`save-state`, and :samp:`restore-state`.
A useful SDK rarely needs all five.


setup-base
~~~~~~~~~~

:samp:`setup-base` runs as root
when the SDK is first installed in a workshop
and on every :command:`workshop refresh`.
It is the place for system-wide configuration:
installing :program:`apt` packages,
wiring :envvar:`PATH`,
and laying down service unit files.

A minimal :file:`setup-base` for a single-binary SDK
adds the SDK's :file:`bin/` directory to the system :envvar:`PATH`:

.. code-block:: shell
   :caption: hooks/setup-base

   cat <<EOF > /etc/profile.d/<NAME>.sh
   export PATH="$SDK/bin:\$PATH"
   EOF


Place system-wide environment variables under :file:`/etc/profile.d/`
so they apply across shells.
Avoid editing :file:`/etc/bash.bashrc` directly;
|ws_markup| may support more than one shell
and :file:`/etc/profile.d/` is the portable seam.

Inside :samp:`setup-base`,
:program:`apt` is preconfigured to skip recommended and suggested packages
and to answer "yes" to confirmation prompts,
so :command:`apt-get install` calls can be terse:

.. code-block:: shell
   :caption: hooks/setup-base

   apt-get update
   apt-get install build-essential cmake ninja-build


Operations performed in :samp:`setup-base` become part of the workshop's
:ref:`base snapshot <exp_workshop_definition_sdks>`,
so subsequent refreshes start from a warmed-up state.


setup-project
~~~~~~~~~~~~~

:samp:`setup-project` runs as the :samp:`workshop` user
after :samp:`setup-base`,
once the project directory is mounted
and interface plugs and slots are connected.
This is the right place for per-user configuration:
activating virtual environments,
enabling user :program:`systemd` services,
and writing files under :file:`/home/workshop/`.

A typical :file:`setup-project` for an SDK that ships a service unit
installs and starts the service as a user-level systemd unit:

.. code-block:: shell
   :caption: hooks/setup-project

   install -D --mode=644 --target-directory ~/.config/systemd/user \
       "$SDK/<NAME>.service"

   systemctl --user daemon-reload
   systemctl --user enable --now <NAME>


User-level :program:`systemd` services are preferred over root-level ones
because they cleanly tie their lifetime
to the :samp:`workshop` user's session
and don't require :samp:`sudo`.

Operations in :samp:`setup-project` don't go into the base snapshot,
so use it for anything that depends on project-specific state
or that should be re-evaluated on every launch.


Other hooks
~~~~~~~~~~~

The other three hooks,
:samp:`check-health`, :samp:`save-state`, and :samp:`restore-state`,
report SDK health and carry state across a refresh.
For elaborate examples of all five hooks,
see :ref:`how_write_runtime_hooks`.


.. _how_build_sdk_try:

Try the SDK
-----------

Once the definition and hooks are in place,
build and install the SDK into a workshop with :command:`sdkcraft try`:

.. code-block:: console

   $ sdkcraft try


|sdk_markup| packs the SDK for each declared platform
into files of the form :file:`<NAME>_<ARCH>_<BASE>.sdk`
and copies them into the try area.

Add the SDK to a workshop definition
using the :samp:`try-` prefix:

.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: try-<NAME>


The :samp:`base` must match one of the SDK's :samp:`platforms`.
Then launch the workshop with verbose output
and a wait-on-error breakpoint
so that any hook failure leaves a usable container behind for inspection:

.. code-block:: console

   $ workshop launch --verbose --wait-on-error


Pay particular attention to:

- Hook output in :command:`workshop changes` and :command:`workshop tasks`.

- The SDK's :samp:`status` in :command:`workshop info`;
  a :samp:`waiting` or :samp:`error` state
  is the SDK telling you something is wrong.

- The interaction between this SDK and any other SDKs
  it's meant to be installed alongside.


On success,
:command:`workshop info` reports the SDK
and a :samp:`status` of :samp:`okay`.
On failure,
:command:`workshop changes` and :command:`workshop tasks`
point at the hook that failed;
see :ref:`how_debug_issues_workshops` for the full troubleshooting flow.


Test the SDK
------------

If the SDK ships a :file:`tests/` directory with
`spread <https://github.com/canonical/spread>`__ tests,
run them against the freshly packed artifacts:

.. code-block:: console

   $ sdkcraft test


|sdk_markup| provisions a clean LXD container for each test,
installs the packed SDK into a workshop,
and runs the declared scenarios end-to-end.

Tests live under :file:`tests/`,
organised in suites declared by :file:`tests/spread.yaml`.
The starter test at :file:`tests/main/launch/` illustrates the layout;
add more tests next to the starter,
each in its own subdirectory of the same suite:

.. code-block:: yaml
   :caption: tests/main/smoke/task.yaml

   summary: SDK installs and reports healthy
   execute: |
     workshop launch --verbose --wait-on-error
     workshop info | grep -E 'status:\s+okay'


Iterate
-------

Normally, you would use the :command:`workshop sketch-sdk` command
to iterate on an SDK locally.
However, even when it doesn't fit your purpose,
the build-try-fix loop is fast:

#. Edit the definition or a hook.

#. Run :command:`sdkcraft clean && sdkcraft try`
   to rebuild from a clean state.

#. Run :command:`workshop refresh`
   to reapply the SDK in the existing workshop,
   or :command:`workshop launch --verbose --wait-on-error`
   for a fresh start.


:command:`sdkcraft clean` is optional;
omit it when the change is small enough
that |sdk_markup| can incrementally rebuild.
For build internals, see the Craft Parts
`lifecycle documentation
<https://documentation.ubuntu.com/craft-parts/latest/common/craft-parts/explanation/lifecycle/>`_.


Next steps
----------

When the SDK behaves correctly under :command:`sdkcraft try`
and its test suite passes,
proceed to :ref:`how_publish_sdk`
to register the SDK name on the SDK Store and upload a revision.


See also
--------

Explanation:

- :ref:`exp_best_interfaces`
- :ref:`exp_best_parts_decomposition`
- :ref:`exp_best_parts_or_hooks`
- :ref:`exp_interfaces`
- :ref:`exp_sdk_best_practices`
- :ref:`exp_sdk_concepts`
- :ref:`exp_sdk_hooks`
- :ref:`exp_sdk_parts`
- :ref:`exp_test_try_sdk`
- :ref:`exp_workshopctl`
- :ref:`exp_workshop_definition_sdks`


How-to guides:

- :ref:`how_declare_plugs_slots`
- :ref:`how_write_runtime_hooks`


Reference:

- :ref:`ref_ai_design_sdk_skill`
- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_hooks`
- :ref:`ref_sdk_parts`
- :ref:`ref_sdkcraft_definition`
- :ref:`ref_workshop_sketch-sdk`
- :ref:`ref_workshopctl__cli`


Tutorial:

- :ref:`tut_craft_sdks`
