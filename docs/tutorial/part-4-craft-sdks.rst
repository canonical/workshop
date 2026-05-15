.. _tut_craft_sdks:

.. meta::
   :description: Tutorial on crafting SDKs with SDKcraft, teaching users how to
                 initialize, define, pack, and publish SDKs for sharing and use
                 with the CLI utility.

Craft SDKs with |sdk_markup|
==============================

.. @tests in tests/docs-tutorial/part-4/task.yaml

.. @artefact sdkcraft (CLI)
.. @artefact SDK
.. @artefact interface

This is the fourth section of the :ref:`four-part series <tut_index>`;
you'll learn how to create full-featured SDKs
that can be published and shared with others using |sdk_markup|.
It relies on the knowledge gained in the :ref:`tut_get_started` section,
where you learned how to create and run workshops,
and also builds on the :ref:`tut_sketch_sdks` section,
where you learned how to sketch local SDKs.

Here, you will initialize, define, pack, and publish an :ref:`SDK <exp_sdks>`:
a set of hooks, interfaces, and parts that is bundled into a single package,
suitable for use with |sdk_markup|, the user-oriented CLI utility.
The commands you're about to run
cover most of your daily needs with |sdk_markup|.


Install |sdk_markup|
--------------------

Install the snap using the
`--classic <https://snapcraft.io/docs/install-modes/>`_ option:

.. code-block:: console

   $ sudo snap install --classic sdkcraft

Prerequisites
~~~~~~~~~~~~~

|sdk_markup| relies on
`LXD 6.8+ <https://canonical.com/lxd>`_
for low-level operation,
using its
`REST API <https://documentation.ubuntu.com/lxd/latest/restapi_landing/>`_
to craft the SDKs.

If the :command:`snap install` command reports an issue with LXD,
install a recent LXD version with :program:`snap`.

To install it from scratch:

.. code-block:: console

   $ sudo snap install --channel=6/stable lxd


To refresh an existing installation:

.. code-block:: console

   $ sudo snap refresh --channel=6/stable lxd


.. note::

   For other ways to install LXD,
   see the available installation options in
   `LXD documentation
   <https://documentation.ubuntu.com/lxd/latest/installing/>`_.
   Also, you need to ensure the
   `LXD daemon
   <https://documentation.ubuntu.com/lxd/latest/explanation/lxd_lxc/#lxd-daemon>`_
   is enabled and running.
   Again, refer to LXD documentation
   and your distribution's manuals for guidance.


.. _tut_sdkcraft_init:

Initialize the SDK
------------------

Once you have installed |sdk_markup|,
use it to initialize, define, and pack your first :ref:`SDK <exp_sdks>`.
Here, we'll build an SDK that installs Ollama
for running large language models in the workshop.
This demonstrates creating an SDK for a specific application,
but SDKs can package any software that aligns with the |ws_markup| way.

First, create a directory named :file:`ollama/`:

.. code-block:: console

   $ mkdir ollama/


.. @artefact SDK definition

It will contain your :ref:`SDK definition <exp_sdk_definition>`
and other source files.

Next, browse to the SDK directory and initialize it:

.. code-block:: console

   $ cd ollama/
   $ sdkcraft init


This command creates a template definition file
named :file:`sdkcraft.yaml`;
although it's almost empty,
it can already be :ref:`built <tut_sdkcraft_try>`.

However, let's take a few extra steps
to explore what goes into an SDK.


Update metadata
---------------

Update the metadata in :file:`sdkcraft.yaml`
with some domain-specific information
to describe the project
and build SDKs for several platforms:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   name: ollama
   version: "0.9.6"
   summary: Get up and running with large language models
   description: |
     Get up and running with Llama 3.3, DeepSeek-R1, Phi-4,
     Gemma 3, Mistral Small 3.1 and other large language models.
   license: MIT
   platforms:
     ubuntu@22.04:amd64:
     ubuntu@24.04:amd64:

   parts:
     my-part:
       plugin: nil


.. _tut_sdkcraft_parts:

Define parts
------------

|sdk_markup| leverages the :ref:`parts mechanism <exp_sdk_parts>`
to obtain data from different sources, process it in various ways,
and prepare an SDK package for publishing.

In our Ollama SDK, we'll define two parts:
one to download the Ollama binary from its GitHub release page,
and another for the :program:`systemd` service file:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   # ...
   parts:
     ollama:
       plugin: dump
       source: https://github.com/ollama/ollama/releases/download/v${CRAFT_PROJECT_VERSION}/ollama-linux-amd64.tgz
       source-type: tar
     user-service:
       plugin: dump
       source: ollama.service
       source-type: file


The :samp:`ollama` part uses the :samp:`dump` plugin
to download and extract the official Ollama binary from GitHub releases.

The :samp:`user-service` part includes a :program:`systemd` service file
that will be used to manage the Ollama daemon.

The first :samp:`dump` downloads the file automatically.
However, we need to create the :program:`systemd` service file
that was referenced in the :samp:`user-service` part.

In the :file:`ollama/` directory,
create a file named :file:`ollama.service`:

.. code-block:: ini
   :caption: ollama.service

   [Unit]
   Description=Ollama Service
   After=network.target

   [Service]
   ExecStart=/bin/bash -lc "ollama serve"
   Restart=always
   RestartSec=3

   [Install]
   WantedBy=default.target


This defines how the Ollama daemon should run:

- :samp:`ExecStart` starts the server with a login shell
  to pick up the environment

- :samp:`Restart` ensures the service is restarted on failure

- :samp:`After` makes it depend on network connectivity


.. note::

   The service file is specific to Ollama and how it runs as a daemon.
   This is just one way to manage a long-running process in an SDK,
   and other SDKs may use different part layouts depending on their needs.

   For in-depth details,
   refer to the `Parts
   <https://documentation.ubuntu.com/craft-parts/latest/common/craft-parts/explanation/parts/>`_
   section in Craft Parts documentation.


.. _tut_sdkcraft_mount_interface:

Add plugs and slots
-------------------

.. @artefact interface plug

In |sdk_markup|,
:ref:`interfaces <exp_interface_concepts>` provide a controllable way
of exposing the resources of the host system to the workshops,
and you can use them in a variety of ways
to extend the functionality of your SDK.

For the Ollama SDK, we need several interfaces:
a mount interface to preserve models,
a GPU interface for acceleration,
and a tunnel interface to expose the API server.
The latter is a resource that the SDK itself exposes,
so it will be defined as a slot;
the former two are plugs because they access external resources.

Open :file:`sdkcraft.yaml` again
and add two plugs and a slot to the appropriate sections:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 12-22

   name: ollama
   version: "0.9.6"
   summary: Get up and running with large language models
   description: |
     Get up and running with Llama 3.3, DeepSeek-R1, Phi-4,
     Gemma 3, Mistral Small 3.1 and other large language models.
   license: MIT
   platforms:
     ubuntu@22.04:amd64:
     ubuntu@24.04:amd64:

   plugs:
     gpu:
       interface: gpu
     models:
       interface: mount
       workshop-target: /home/workshop/.ollama/models

   slots:
     ollama-server:
       interface: tunnel
       endpoint: 11434

   # ...


The :samp:`models` plug preserves downloaded models between workshop refreshes.
The :samp:`gpu` plug provides access to GPU acceleration for faster inference,
and the :samp:`ollama-server` slot exposes the Ollama API on port 11434.

.. note::

   You can't explicitly set the *host* directory for mount plugs here;
   this restriction prevents SDKs
   from accessing any arbitrary data on the host filesystem.
   However, users who add your SDK to their workshops
   will be able to remount the plug elsewhere at runtime.


Add hooks
---------

.. @artefact SDK hook

To prepare the SDK for use,
add the :ref:`hooks <exp_sdk_hooks>`
that run at different stages of the workshop's lifecycle,
preparing the SDK for use or preserving its state during updates.

Under :file:`ollama/`,
there is a subdirectory
named :file:`hooks/`.
This directory stores all the hooks for an SDK.


Build: setup base, project
~~~~~~~~~~~~~~~~~~~~~~~~~~

.. @artefact setup-base
.. @artefact setup-project

Under :file:`ollama/hooks/`,
edit the file
named :file:`setup-base`:

.. code-block:: shell
   :caption: setup-base

   cat <<EOF >/etc/profile.d/ollama.sh
   export PATH="$SDK/bin:\$PATH"
   EOF


It runs when the workshop is launched or refreshed,
and is typically used to install system packages
and configure the environment.

In the same directory,
edit the file named :file:`setup-project`
for Ollama-specific setup:

.. code-block:: shell
   :caption: setup-project

   install -D --mode=644 --target-directory ~/.config/systemd/user "$SDK/ollama.service"

   systemctl --user daemon-reload
   systemctl --user enable --now ollama


It runs after :file:`setup-base`,
once the project directory is mounted
and interfaces are connected.
This hook installs and starts the Ollama service as a user service,
ensuring the AI model server is running and ready to use.

.. note::

   |ws_markup| tweaks this hook's environment a bit.

   First, note the :envvar:`$SDK` variable,
   which points to the root of the SDK installation.
   This allows you to reference files installed by the SDK.

   Also, when invoked from any hooks,
   :command:`apt` is configured to exclude recommended or suggested packages
   and answer "yes" to all confirmation prompts.


Persist: save and restore
~~~~~~~~~~~~~~~~~~~~~~~~~

.. @artefact restore-state
.. @artefact save-state

Some SDKs need to preserve internal state during workshop refresh operations,
such as configuration settings or temporary data that shouldn't be lost.
For these cases,
you would create :file:`save-state` and :file:`restore-state` hooks.

During a :command:`workshop refresh` operation:

- The :file:`save-state` hook runs *before* the workshop is refreshed,
  saving the state of the SDK to :envvar:`$SDK_STATE_DIR`.

- The :file:`restore-state` hook recovers the state
  *after* the workshop has been successfully updated.


However, the Ollama SDK doesn't need these hooks because:

- Downloaded models are stored in the mounted :file:`models/` directory,
  which persists across refreshes

- The :program:`systemd` service configuration
  is stateless and recreated on each refresh

- No custom user configuration needs to be preserved


.. warning::

   The SDK is also refreshed as a part of any workshop refresh operation,
   so any breaking changes in its save-restore logic will cause an error;
   make sure to allow for this in your SDK design.


Report: check health
~~~~~~~~~~~~~~~~~~~~

.. @artefact check-health

Finally, create a hook named :file:`check-health`
to test whether the installation is functional
and report to |ws_markup| accordingly:

.. @artefact workshopctl

.. code-block:: shell
   :caption: check-health

   if ! output=$(sudo -u workshop --login ollama list 2>&1); then
     workshopctl set-health error "$output"
     exit
   fi
   workshopctl set-health okay


It checks whether the Ollama installation is functional
by running :command:`ollama list`.
If it succeeds, the health is set to :samp:`okay`
using the :ref:`workshopctl set-health <exp_workshopctl>` command;
otherwise, it reports the error output from the failed command.

In general, the hook should set the health to :samp:`okay`
and return a zero code if its health checks succeed.
To signal an error, set the health to :samp:`error`
or return a nonzero code.

You can also set the health to :samp:`waiting`
to signal that the hook should be retried for a few seconds.
Unless the hook sets the health to a different value during such a retry,
the health is eventually set to :samp:`error` automatically.

.. note::

   The use of :command:`sudo -u workshop` here is important
   because only the :samp:`setup-project` hook runs as a normal user by default;
   other hooks, like :samp:`check-health`, run as root.
   Running commands as the nonroot user
   helps preserve the correct environment variables and file ownership,
   and can be easier than adjusting permissions afterwards.


.. _tut_sdkcraft_try:

Try the SDK
-----------

.. @artefact sdkcraft (CLI)

When you're confident the SDK is ready to be built,
try it in-place before uploading it to the Store.

Under :file:`ollama/`, run:

.. code-block:: console

   $ sdkcraft try

     Packed ollama_amd64_ubuntu@22.04.sdk
     Packed ollama_amd64_ubuntu@24.04.sdk
     ...


Optionally, you can clean the build cache before trying:

.. code-block:: console

   $ sdkcraft clean && sdkcraft try


.. @artefact SDK part
.. @artefact SDK metadata

The command builds and packs the SDK into files
such as :file:`ollama_amd64_ubuntu@24.04.sdk`,
which contain the build artifacts
along with SDK metadata, hooks, and other components.
This is repeated for all supported :samp:`platforms`
defined in the :file:`sdkcraft.yaml` metadata.

In particular, the command builds all :ref:`SDK parts <exp_sdk_parts>`
defined in the :file:`sdkcraft.yaml` file,
e.g., pulling source code, applying patches, configuring and compiling it
according to the part definition.

After a successful build,
the :command:`sdkcraft try` command also copies the SDKs to a special *try area*
(usually :file:`$XDG_DATA_HOME/workshop/try/`).
To use them in a workshop, add a prefix: :samp:`try-<NAME>`:

.. code-block:: yaml
   :caption: .workshop/dev.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: try-ollama


A :samp:`channel` is not needed here;
the SDK is installed from the try area when you launch the workshop;
the options :option:`!--verbose` and :option:`!--wait-on-error`
help debug any issues that may arise during launch or refresh:

.. code-block:: console

   $ workshop launch --verbose --wait-on-error


.. note::

   For a detailed explanation of the build process,
   see the respective Craft Parts
   `documentation section
   <https://documentation.ubuntu.com/craft-parts/latest/common/craft-parts/explanation/lifecycle/>`_.


.. _tut_sdkcraft_test:

Test the SDK
------------

Additionally,
you can write and run `spread <https://github.com/canonical/spread>`__ tests
against the SDK to ensure its functionality
and catch any issues before publishing it.
For |sdk_markup|, :program:`spread` tests are declared
under :file:`tests/` in the SDK directory;
each test describes a specific executable user workflow.

To run the test suite against the packed SDK:

.. code-block:: console

   $ sdkcraft test


At runtime, each test provisions a clean LXD container,
installs the packed SDK into a workshop,
and runs the declared scenarios end-to-end.


.. _tut_sdkcraft_publish:

Publish the SDK
---------------

.. @artefact SDK Store

When an SDK is ready, built, and tried,
publish it to the SDK Store
for use with |ws_markup|.

Authenticate, register the SDK name, and upload the artifact:

.. code-block:: console

   $ sdkcraft login
   $ sdkcraft register ollama
   $ sdkcraft upload ./ollama_amd64_ubuntu@24.04.sdk --release latest/beta


This uploads the newly created SDK
and releases it under the :samp:`latest/beta` channel in the SDK Store.

For the full publish flow, including how to release
already-uploaded revisions to additional channels,
see :ref:`how_publish_sdk`.


Use the SDK
-----------

The resulting SDK can be used with |ws_markup| as follows:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 4,5

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: latest/beta


Note that the workshop :samp:`base`
must match one of the SDK's supported :samp:`platforms`.


Summary
-------

This was the last step of the entire tutorial series.

You have learned how to create a workshop,
add SDKs to it, and use them in practice.
You have also learned how to sketch a local SDK
and how to craft and publish a full-featured SDK.
You are now familiar with all the basic operations
that |ws_markup| and |sdk_markup| provide
and have had an extensive tour of their capabilities.
