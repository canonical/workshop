.. _exp_sdk_best_practices:

.. meta::
   :description: Best practices for crafting SDKs in Workshop, explaining design
                 decisions for system services, parts decomposition, interfaces,
                 environment variables, content delivery, and health checks.

Design best practices
=====================

.. @artefact SDK
.. @artefact SDK part
.. @artefact SDK hook
.. @artefact setup-base
.. @artefact setup-project
.. @artefact check-health

When crafting SDKs for |ws_markup|,
publishers face design decisions
that affect how their SDKs install, integrate, and work inside workshops.
Understanding the best practices outlined below
helps publishers create more maintainable, reliable, and user-friendly SDKs
that better align with |ws_markup|'s architecture and ideology.

This explanation covers key design considerations
and provides rationale for common patterns found in a number of SDKs
available in the |ws_markup| ecosystem.


.. _exp_best_services:

System services
---------------

System services within SDKs should be designed to integrate smoothly
with the workshop's lifecycle and other SDK components.

Consider the approach used by the :samp:`ollama` SDK:
it implements a :samp:`setup-project` hook
that configures and starts the :program:`systemd` service
by including a service file:

.. code-block:: yaml
   :caption: ollama/sdkcraft.yaml

   parts:
     user-service:
       plugin: dump
       source: ollama.service
       source-type: file


The file provides appropriate service configuration:

.. code-block:: ini
   :caption: ollama/ollama.service

   [Unit]
   Description=Ollama Service
   After=network.target

   [Service]
   ExecStart=/bin/bash -lc "ollama serve"
   Restart=always
   RestartSec=3

   [Install]
   WantedBy=default.target


And gets installed during the :samp:`setup-project` phase:

.. code-block:: shell
   :caption: ollama/hooks/setup-project

   install -D --mode=644 --target-directory ~/.config/systemd/user "$SDK/ollama.service"

   systemctl --user daemon-reload
   systemctl --user enable --now ollama


This design ensures that the service starts automatically
when the workshop is launched,
and stops cleanly when the workshop is terminated.


.. _exp_best_parts_decomposition:

Parts decomposition
-------------------

The :ref:`parts mechanism <exp_sdk_parts>`,
shared by |ws_markup| with projects such as `Snapcraft <https://documentation.ubuntu.com/snapcraft/stable/explanation/parts/>`_,
enables modularity by separating different aspects of an SDK
into discrete, manageable components.
Effective decomposition strategies depend on the SDK's complexity
and the independence of its components.

Consider the :samp:`go` SDK, which uses a single part
because the Go toolchain can be distributed as a cohesive unit:

.. code-block:: yaml
   :caption: go/sdkcraft.yaml

   parts:
     go:
       plugin: dump
       source: https://go.dev/dl/go$CRAFT_PROJECT_VERSION.linux-$CRAFT_ARCH_BUILD_FOR.tar.gz
       source-type: tar


In contrast, the :samp:`ollama` SDK is built with multiple parts
for the runtime and service configuration,
allowing selective updates and reducing build times:

.. code-block:: yaml
   :caption: ollama/sdkcraft.yaml

   parts:
     ollama:
       plugin: dump
       source: https://github.com/ollama/ollama/releases/download/v0.9.6/ollama-linux-amd64.tgz
       source-type: tar
     user-service:
       plugin: dump
       source: ollama.service
       source-type: file


Parts should be organized around functional boundaries:

.. list-table::
  :header-rows: 1
  :widths: 25 75

  * - Component Type
    - Description

  * - Runtime components
    - Core binaries and libraries that change infrequently

  * - Configuration
    - Settings and templates that may need customization

  * - Data assets
    - Large files like models or datasets that update independently

  * - Tools
    - Auxiliary utilities that complement the main functionality


However, parts are not mandatory:
the minimal viable option is to forgo them entirely
and install everything in the :ref:`hooks <exp_best_parts_or_hooks>`.


.. _exp_best_interfaces:

Interface layout
----------------

Interfaces define how SDKs interact with the host system and other SDKs.
The layout of interfaces ultimately impacts an SDK's usability and security.
Publishers should select interfaces
based on the resources their SDK requires (via plugs) or exposes (via slots).

In particular,
the :ref:`mount interface <exp_mount_interface>` plugs are frequently used
because they specifically address data persistence and sharing needs.
The :samp:`uv` SDK demonstrates this by mounting :file:`/home/workshop/.cache/uv`
to preserve package caches across workshop life-cycle,
improving performance for `workshop refresh`:

.. code-block:: yaml
   :caption: uv/sdkcraft.yaml

   plugs:
     cache:
       interface: mount
       workshop-target: /home/workshop/.cache/uv


This configuration means that the :file:`/home/workshop/.cache/uv/` directory
inside the workshop maps to a persistent storage location on the host system.
This setup allows the :samp:`uv` SDK to retain its cache between refreshes.

Rather than a plug, a slot provides resources to the workshop;
for instance, the :samp:`ollama` SDK uses the :samp:`tunnel` interface slot
to expose its server functionality on a specific port,
enabling external access to its services:

.. code-block:: yaml
   :caption: dotnet10/sdkcraft.yaml

   slots:
     ollama-server:
       interface: tunnel
       endpoint: 11434


The most obvious interface choices are as follows:

- Use :samp:`mount` for persistent data and caches
- Use :samp:`gpu` when GPU acceleration is required
- Use :samp:`tunnel` for network services that need to be accessible externally
- Use :samp:`ssh` for authentication with remote services


For a complete list, see :ref:`exp_interfaces`;
for a discussion of interface capabilities, see :ref:`exp_interface_concepts`.


Environment variables
---------------------

Environment variables provide a clean way
to configure SDK behavior and integrate with workshops.
SDKs should use standard POSIX-compatible shell mechanisms
to add variables to the workshop.

For system-wide variables that affect all users,
SDKs should place configuration files in :file:`/etc/profile.d/`.
The :samp:`uv` SDK demonstrates this approach
by setting system-wide :envvar:`PATH` modifications
in its :samp:`setup-base` hook:

.. code-block:: shell
   :caption: uv/hooks/setup-base

   cat <<EOF >> /etc/profile.d/uv.sh
   PATH="$SDK/bin:\$PATH"
   EOF


For user-specific variables that only make sense for the :samp:`workshop` user,
SDKs should modify :file:`~/.profile`.
Again, the :samp:`uv` SDK illustrates this pattern
by setting :envvar:`UV_LINK_MODE=copy` in its :samp:`setup-project` hook
to address interaction between SDK behavior and workshop architecture:

.. code-block:: shell
   :caption: uv/hooks/setup-project

   cat <<EOF >> ~/.profile

   # SDK uses 'mount' interface to preserve
   # uv cache across refreshes, thus, hardlinking is
   # not available on 'uv sync'.
   export UV_LINK_MODE=copy
   EOF


Publishers should avoid shell-specific configuration files,
such as :file:`~/.bash_profile` or :file:`~/.bashrc`,
because |ws_markup| supports multiple shell interpreters
and these files may not be sourced consistently
across different shell sessions.

Some guidelines for environment variables:

- Use clear names that indicate origin and purpose;
  prefix them with the SDK name to avoid conflicts
- Include comments explaining why specific values are chosen
- Choose between :file:`/etc/profile.d/` for system-wide settings
  and :file:`~/.profile` for user-specific configuration


.. _exp_best_parts_or_hooks:

Parts or hooks?
---------------

The decision between shipping prebuilt content
and, alternatively, installing it dynamically at runtime through hooks
affects SDK size, startup time, and flexibility.
Different content types have different optimal strategies.

For instance, Debian packages are best installed in hooks,
particularly in :samp:`setup-base`,
because they integrate with the system package manager
and can leverage :program:`apt`'s local cache.
Installing packages during SDK build
would bypass distribution security updates
and create larger SDK artifacts.

The :samp:`ros2` SDK exemplifies this approach:

.. code-block:: shell
   :caption: ros2/hooks/setup-base

   apt-get update
   apt-get install ros-dev-tools
   apt-get install python3-colcon-argcomplete python3-colcon-alias python3-colcon-clean python3-colcon-mixin
   # ...


In general, binary artifacts are best shipped as parts when you need to:

- Pin specific versions regardless of what's available in package repositories,
- Distribute custom builds with specialized compilation flags,
- Provide tools that aren't available through the system package manager.


This approach ensures consistent environments
and avoids eventual dependency conflicts.

The :samp:`uv` SDK shows this approach by shipping prebuilt Rust binaries:

.. code-block:: yaml
   :caption: uv/sdkcraft.yaml

   parts:
     uv:
       plugin: rust
       source: https://github.com/astral-sh/uv
       source-tag: $CRAFT_PROJECT_VERSION
       source-type: git
       organize:
         uv: bin/uv
         uvx: bin/uvx
       prime:
         - bin/uv
         - bin/uvx


:samp:`setup-base` or :samp:`setup-project`?
--------------------------------------------

The choice between :samp:`setup-base` and :samp:`setup-project` hooks
fundamentally affects when and how SDK initialization occurs.
This decision impacts performance, caching behavior,
and the SDK's presence in workshop snapshots.

First of all,
note that both :samp:`setup-base` and :samp:`setup-project`
should configure the workshop for running,
but normally don't directly control service startup or other runtime behavior.
For instance, they can configure container shutdown or startup,
but they shouldn't start services directly
unless there's a specific reason to do so.

The :samp:`setup-base` hook runs once when the SDK is installed
at launch or refresh time,
making it ideal for system-wide configuration
that doesn't change between projects.
Operations in :samp:`setup-base` become part of
:ref:`workshop snapshots <exp_workshop_definition_sdks>`,
improving startup performance at subsequent refreshes.

For instance,
the :samp:`uv` SDK uses :samp:`setup-base` for system-wide configuration
that includes shell completion, :envvar:`PATH` updates,
and system package manager integration:

.. code-block:: shell
   :caption: uv/hooks/setup-base

   sudo -u workshop mkdir -p /home/workshop/uv-venv

   cat <<EOF >> /etc/profile.d/uv.sh
   PATH="$SDK/bin:\$PATH"
   EOF

   "$SDK"/bin/uv generate-shell-completion bash > /etc/bash_completion.d/uv.sh
   "$SDK"/bin/uvx --generate-shell-completion bash > /etc/bash_completion.d/uvx.sh

   mkdir -p /usr/local/libexec/alternatives

   cat << 'EOF' > /usr/local/libexec/alternatives/uv-pip
   #!/bin/bash
   exec uv pip "$@"
   EOF

   chmod +x /usr/local/libexec/alternatives/uv-pip
   update-alternatives --install /usr/bin/pip pip /usr/local/libexec/alternatives/uv-pip 50


The :samp:`setup-project` hook runs as the :samp:`workshop` user
after :samp:`setup-base`,
when interfaces are connected and the workshop is fully operational.
This makes it suitable for project-specific initialization
that might vary depending on the actual project files.

For instance, the :samp:`comfy` SDK uses :samp:`setup-project`
to detect the available GPU type,
configuring the appropriate PyTorch variant accordingly:

.. code-block:: shell
   :caption: comfy/hooks/setup-project

   GPU_TYPE="none"

   if command -v lspci >/dev/null 2>&1; then
       if lspci | grep -i 'NVIDIA' >/dev/null 2>&1; then
           GPU_TYPE="nvidia"
       elif lspci | grep -i 'AMD/ATI' >/dev/null 2>&1; then
           GPU_TYPE="amd"
       elif lspci | grep -i 'Intel.*Graphics' >/dev/null 2>&1; then
           GPU_TYPE="intel"
       fi
   fi

   echo "Detected GPU: $GPU_TYPE"


The need to use :samp:`setup-project` for this purpose arises
from the fact that the GPU is accessed via an auto-connected interface,
so its availability can only be determined
after the workshop has launched and interfaces are connected.
However, the choice of packages to install depends on the GPU type,
necessitating dynamic configuration at project setup time:

.. code-block:: shell
   :caption: comfy/hooks/setup-project

   case "$GPU_TYPE" in
       nvidia)
           pip install torch torchvision torchaudio --extra-index-url https://download.pytorch.org/whl/cu129
           ;;
       amd)
           pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/rocm6.4
           ;;
       *)
           pip install torch torchvision torchaudio
           ;;
   esac


In general, you use :samp:`setup-base` for:

- System package installation
- Global environment configuration
- One-time setup operations
- Content that should be part of snapshots
  (e.g., infrequently updated or unlikely to change)


Choose :samp:`setup-project` for:

- Project-specific configuration that depends on the project context
- Operations requiring :ref:`auto-connected interfaces <exp_interface_connections>`
- Content that shouldn't be part of snapshots
  (e.g., frequently updated or extra large)


.. _exp_best_health_checks:

Health checks
-------------

Health check scripts provide essential feedback
about SDK operational status and help users diagnose problems quickly.
Well-designed health checks go beyond simple binary success or failure,
reporting extra details to provide actionable diagnostic information.

The :samp:`ollama` SDK demonstrates comprehensive health checking
by testing actual service functionality and channeling its error output:

.. code-block:: shell
   :caption: ollama/hooks/check-health

   if ! output=$(sudo -u workshop --login ollama list 2>&1); then
     workshopctl set-health error "$output"
     exit
   fi
   workshopctl set-health okay


When the workshop is launched with :option:`!--wait-on-error`,
the :command:`workshop info` output will contain these details.

In general, health checks should:

- Test each of the relevant features, not just the sheer fact of installation

- Provide specific error codes for different failure modes

- Include helpful error messages that guide troubleshooting
  with :command:`workshop changes`

- Run quickly to avoid slowing workshop operations down

- Handle edge cases gracefully


.. _exp_best_dependencies:

SDK dependencies
----------------

The :samp:`mount` interface enables sophisticated collaboration patterns
between SDKs within a workshop while avoiding explicit dependency management.
Rather than having each SDK prepare and maintain its own resources,
SDKs can expose capabilities they provide through slots
and consume them through plugs,
creating efficient resource utilization patterns.

Consider Python-based SDKs that need to install packages from PyPI.
Instead of each SDK maintaining its own virtual environment,
one SDK can provide a shared environment that others consume.
The :samp:`uv` SDK demonstrates this by exposing a virtual environment slot:

.. code-block:: yaml
   :caption: uv/sdkcraft.yaml

   slots:
     venv:
       interface: mount
       workshop-source: /home/workshop/uv-venv


Here, :samp:`workshop-source` says that the resource is inside the workshop,
rather than on the host.

Other Python-based SDKs can then connect to this shared environment
through corresponding plugs.
The :samp:`jupyter` SDK shows this pattern:

.. code-block:: yaml
   :caption: jupyter/sdkcraft.yaml

   plugs:
     venv:
       interface: mount
       workshop-target: $SDK/venv


|ws_markup| users wire the two together
through a :samp:`connections:` block in their workshop definition.
Without that block, the pairing doesn't happen,
however well the plug and the slot match:
the consuming SDK falls back to the host directory
that |ws_markup| automatically provides for the plug,
so a Python-based SDK still works on its own.

This pattern extends beyond Python or its virtual environments
to encompass various shared resources,
including common libraries and runtime environments,
shared data directories and caches,
and development tools and utilities.
It offers several advantages:

- Eliminates duplication of large tool chains or environments
- Maintains separation between SDK responsibilities
- Allows workshop users to mix and match compatible SDKs
- Avoids the complexity of dependency management with a fallback mechanism

Both examples above assume SDK publishers ship the required plugs and slots.
When they don't,
the workshop user can
:ref:`graft <exp_workshop_definition_connections>` the missing elements
in the workshop definition,
extending an SDK's capabilities
without the publisher's involvement.
This makes plug and slot management a shared effort:
SDK authors define the standard capabilities,
and users augment them to fit their project's needs.


See also
--------

Explanation:

- :ref:`exp_interface_concepts`
- :ref:`exp_mount_interface`
- :ref:`exp_sdk_concepts`
- :ref:`exp_sdk_hooks`
- :ref:`exp_sdk_parts`


How-to guides:

- :ref:`how_share_content_between_sdks`


Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_hooks`
- :ref:`ref_sdk_parts`
- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_info`
- :ref:`ref_workshop_start`
- :ref:`ref_workshop_stop`
