.. _exp_arch_system_components:

.. meta::
    :description: Explanation article on the system architecture and components of
                  Workshop, detailing installation, the workshopd daemon, LXD backend,
                  state management, interface policy, systemd integration, and data
                  architecture with workshop launch processes.

System components
=================

|ws_markup| is designed to run on Linux systems
and is primarily distributed as a `snap <https://snapcraft.io/>`_.
It relies on LXD as its containerization backend
and ZFS for efficient storage management.
Each workshop runs inside an LXD system container,
a full operating system environment with its own init system,
rather than an application container that packages a single service
(as with :program:`Docker` or :program:`Podman`).

|ws_markup|'s distributed architecture
organizes functionality across specialized subsystems,
with each handling specific aspects of workshop lifecycle management
while maintaining clear separation of concerns
and well-defined communication interfaces.

The core subsystems include
the :ref:`workshopd daemon <exp_arch_daemon>` for orchestration,
the :ref:`LXD backend <exp_arch_lxd_backend>` for container management,
the :ref:`state database <exp_arch_state_database>` for transactional jobs,
the :ref:`interface system <exp_arch_interface_system>` for resource sharing,
and :program:`systemd` integration for service lifecycle management.
Storage is handled through the :ref:`ZFS storage component <exp_arch_zfs_storage>`
with metadata persistence via the :ref:`state database <exp_arch_state_database>`.

.. note::

   For a description of how these components interact at runtime,
   see :ref:`exp_arch_runtime_behavior`.


Main components
---------------

.. @artefact workshop (CLI)
.. @artefact workshopd
.. @artefact workshopctl

|ws_markup| installation on your host system includes three primary components:

- The :program:`workshop` :ref:`CLI <exp_arch_cli>`
  serves as the main user interface,
  providing a thin client that translates user commands
  and communicates with :program:`workshopd` through a Unix domain socket.

- The :program:`workshopd` :ref:`daemon <exp_arch_daemon>` runs in the background
  with elevated privileges.
  It manages the full workshop lifecycle
  including container creation,
  SDK installation,
  interface coordination,
  and state persistence through a REST API.

- The :program:`workshopctl` :ref:`tool <exp_arch_workshopctl>` runs inside a workshop
  and has access to a very limited subset of :program:`workshopd`'s REST API
  for service-related and reporting tasks.


The communication architecture between these components uses a layered approach
where the CLI communicates with :program:`workshopd` via Unix domain sockets,
while :program:`workshopd` interfaces with LXD through the latter's native API.


.. _exp_arch_cli:

CLI: :program:`workshop`
------------------------

.. @artefact workshop (CLI)

The :program:`workshop` CLI provides a comprehensive command-line interface
for managing workshops and interacting with :program:`workshopd`.
It is organized into logical command groups
for different aspects of |ws_markup|'s operations:

- Create, update, and delete operations
- Start and stop control
- Exploration and troubleshooting
- Interface connection management
- Workshop utilization
- SDK sketching
- Miscellaneous utilities


For all operations, the CLI communicates with :program:`workshopd` through a REST API
exposed over Unix domain sockets,
using a client library that handles connection management,
error handling, and request-response serialization.
The client supports both synchronous and asynchronous operations.

For further details,
see :ref:`the workshop CLI <exp_workshop_cli>`.


.. _exp_arch_workshopctl:

Control interface: :program:`workshopctl`
-----------------------------------------

.. @artefact workshopctl

The :program:`workshopctl` tool serves as a secure bridge
between workshop containers and the host system,
sending control and status messages from inside the workshop back to the host.

The tool is typically invoked by SDK hooks;
it operates with the workshop user's permissions (UID 1000)
and communicates with :program:`workshopd`'s :ref:`REST API <exp_arch_api>`
through a dedicated Unix domain socket inside the workshop
at :file:`/var/lib/workshop/run/workshop.socket.untrusted`.

The tool's capabilities are intentionally limited
to minimize security risks.


.. _exp_arch_daemon:

Daemon: :program:`workshopd`
----------------------------

The :program:`workshopd` daemon serves as the central orchestration hub,
coordinating all workshop operations and maintaining system state.

The daemon exposes the primary REST API for CLI and external integrations
while orchestrating :ref:`workshop <exp_workshop_concepts>` lifecycle
through transactional state management.
It coordinates :ref:`interface connections <exp_interface_concepts>`
and policy validation.

Key components within the daemon include:

- the REST API server for handling HTTP requests,

- the state engine as central coordinator,


- the task runner for running tasks according to their dependencies,

- and specialized state managers for workshops, :ref:`SDKs <exp_sdk_concepts>`,
  :ref:`interfaces <exp_arch_interface_system>`, commands, and hooks.


During startup, the daemon initializes state managers,
establishes an LXD connection,
and starts the API server.
It supports graceful shutdown with task completion and state persistence.
Failure modes are handled through degraded mode operation for LXD unavailability
and error detection for connection issues.
The daemon provides :program:`systemd` notification support
and structured logging for telemetry.

Authentication occurs via Unix domain socket credentials.
Privilege separation exists between trusted and untrusted API endpoints.


.. _exp_arch_api:

REST API
~~~~~~~~

.. @artefact API

The :program:`workshopd` daemon exposes a versioned REST API (v1)
over Unix domain sockets for secure local communication with the CLI.
The API provides endpoints for all workshop operations.

Trusted endpoints (require Unix socket credentials):

- Workshop lifecycle operations (:samp:`/v1/projects/<ID>/workshops`)
- SDK management (:samp:`/v1/sdks`)
- Interface connection management (:samp:`/v1/connections`)
- Change tracking and monitoring (:samp:`/v1/changes`)
- Warning and error reporting (:samp:`/v1/warnings`)


Untrusted endpoints (accessible from within workshops):

- Workshop control interface (:samp:`/v1/workshopctl`)


The API implements proper access control
and supports both synchronous and asynchronous operations.

For data exchange, the API uses JSON with well-defined types
for workshop information, SDK details, and interface connections.


.. _exp_arch_lxd_backend:

LXD backend
~~~~~~~~~~~

.. @artefact workshopd

|ws_markup| uses LXD system containers rather than application containers.
A system container runs a full operating system image
with its own init system (:program:`systemd`)
and can run many processes and services at once,
behaving much like a lightweight virtual machine.
This differs from application containers
such as :program:`Docker` or :program:`Podman`,
which typically run a single process.

These containers are also unprivileged:
the container's root user is mapped through user namespaces
to an ordinary, non-root user on the host,
so root inside a workshop holds no privileges on the host system.

For more on LXD system containers, see the `LXD`_ documentation.

The :program:`workshopd` daemon maintains a persistent connection to LXD
through its Unix domain socket at :file:`/var/snap/lxd/common/lxd/unix.socket`,
managing container operations.

The LXD communication layer handles projects and container lifecycle,
storage management, network configuration,
and device pass-through.

Its responsibilities also include base image management and caching,
providing snapshot and restore capabilities for efficient workshop updates.

|ws_markup| implements user isolation through LXD's project system,
automatically creating dedicated projects for each user
following the naming pattern :samp:`workshop.<USERNAME>`.
Each user project includes a corresponding snapshots project
(:samp:`workshop-snapshots.<USERNAME>`)
used for temporary storage during workshop rebuild operations.


.. note::

   The term "project" in relation to |ws_markup|
   can be used in two unrelated senses:

   - LXD projects, identified by their names
     (e.g., :samp:`workshop.john`).
     These are created at :program:`workshopd`'s request
     to provide isolation in LXD.

   - Workshop projects, identified by |ws_markup|-assigned IDs.
     These are user-defined directories (e.g., :samp:`my-workshop`)
     used to organize and manage workshops (e.g., :samp:`my-workshop`).
     They are referenced in CLI commands and API endpoints.


.. _exp_arch_zfs_storage:

Storage backends
~~~~~~~~~~~~~~~~

|ws_markup| uses ZFS for storage on Linux,
with automatic Btrfs fallback on Windows Subsystem for Linux (WSL).
Storage is managed via LXD and requires a minimum pool size of 5 GiB.

The storage backend manages container root filesystems, workshop-specific data volumes,
cached base images, and snapshots for efficient workshop updates and rollbacks.
This component provides copy-on-write storage utilization, LZ4 compression,
and quota management,
and is utilized by the :ref:`LXD backend <exp_arch_lxd_backend>` for container operations.




.. _exp_arch_state_database:

State database
~~~~~~~~~~~~~~

The :file:`state.json` file is the authoritative database
for workshop metadata, configuration, and operational state.
It enables transactional operations with atomic updates and rollback capabilities.

It uses a model of "changes" (high-level modification to the system state)
and "tasks" (operations with Do/Undo handlers that constitute a change)
to ensure that requests either complete fully or are rolled back.


.. _exp_arch_images:

Images
~~~~~~

|ws_markup| containers are created from base operating system images.
By default, |ws_markup| fetches base images,
such as Ubuntu 22.04 LTS, Ubuntu 24.04 LTS, or Ubuntu 26.04 LTS,
from the official
`Ubuntu cloud image repository <https://cloud-images.ubuntu.com/releases/>`_.



.. _exp_arch_interface_system:

Interfaces
~~~~~~~~~~

This system handles interface connections, enforces security policies,
and also manages the lifecycle of resource connections.

First of all, the interface system validates connections between plugs and slots.
Built-in interface declarations enforcement handles auto- and manual connections.

Key components include the interface repository
serving as a registry of available interface types,
policy validator for enforcing connection rules and security constraints,
connection manager handling connection establishment and teardown,
and security backends responsible for creating LXD profiles of established interface connections.

For further details,
see :ref:`exp_interface_concepts`.

.. note::

   See
   `mount.go
   <https://github.com/canonical/workshop/blob/main/internal/interfaces/builtin/mount.go#L40>`_
   in |ws_markup| source code for an elaborate example.


.. _exp_arch_network:

Network
~~~~~~~

|ws_markup| establishes a dedicated network infrastructure
through the :samp:`workshopbr0` bridge network,
providing isolated networking for workshop containers.

:program:`workshopd` registers this bridge with :program:`systemd-resolved`
as link-specific DNS
and registers :samp:`wp` as a routing-only domain,
so :samp:`*.wp` lookups are directed to the bridge.
Each running workshop registers a canonical name
of the form :samp:`<WORKSHOP>-<PROJECT-ID>.wp`,
and :program:`workshopd` adds two aliases that point to it:
:samp:`<WORKSHOP>.<PROJECT-ID>.wp`,
which also serves :samp:`<PROJECT-ID>.wp` as a search domain inside workshops
so a bare :samp:`<WORKSHOP>` resolves,
and a friendlier :samp:`<WORKSHOP>.<PROJECT>.wp`
when the project directory name encodes to a valid, unused DNS label.
Otherwise the workshop keeps the ID-based forms;
see :ref:`exp_workshop_hostname` for the fallback behavior.

Resolving a hostname isn't the same as trusting what you connect to:
DNS resolution over :samp:`workshopbr0` is unauthenticated
and scoped to the bridge network.
For interactive access, |ws_markup| also maintains
a per-user SSH certificate authority
that signs a host certificate for every workshop
and a user certificate for connecting to them.
Clients trust any :samp:`*.wp` host key signed by this authority,
so tools such as :program:`ssh` and VS Code Remote-SSH
connect without a first-connection host-key prompt;
the trust covers the :samp:`*.wp` namespace as a whole
rather than binding to an individual hostname.
See the :doc:`Security policy </security>`
for the SSH trust model in full.


Diagrams
--------

The system components and their interactions:

.. mermaid::
    :alt: System diagram showing the main architectural components in Workshop.
          The CLI component communicates with the workshopd daemon,
          which orchestrates state management, interface resolution,
          and LXD backend operations.
          The systemd integration provides service lifecycle management,
          while ZFS storage handles persistent data and snapshots.
    :align: center
    :zoom:

    graph TB
        subgraph HOST ["Host system"]
            subgraph CLI_SUBSYSTEM ["CLI"]
                cli[Workshop CLI]
                client[HTTP client library]
            end

            subgraph LXD ["LXD"]
                lxd_daemon[LXD daemon]
                containers[(Workshop containers)]
            end

            subgraph DAEMON_SUBSYSTEM ["workshopd"]
                api[REST API server]
                daemon[Daemon]
                state_mgrs[State managers]
                task_runner[Task runner]
                hook_manager[Hook manager]
                workshop_manager[Workshop manager]
                sdk_manager[SDK manager]
            end

            subgraph BACKEND_SUBSYSTEM ["LXD backend"]
                lxd_backend[LXD backend]
            end

            subgraph STATE_SUBSYSTEM ["State management"]
                state_db[(state.json)]
                checkpointer[Checkpoint handler]
            end

            subgraph INTERFACE_SUBSYSTEM ["Interface management"]
                repo[Interface repository]
                connector[Connection manager]
            end

            subgraph STORAGE_SUBSYSTEM ["ZFS storage"]
                zfs_pool[(ZFS pool)]
                snapshot_mgr[Snapshot manager]
                volume_mgr[Volume manager]
            end

            subgraph SYSTEMD_SUBSYSTEM ["systemd integration"]
                service_unit[workshopd.service]
                socket_activation[Socket activation]
                watchdog[Watchdog]
            end
        end

        subgraph EXT_IMG [Image server]
            img_server[("cloud-images.ubuntu.com")]
        end

        api --> daemon
        cli --> api
        client --> api
        connector --> lxd_backend
        connector --> repo
        daemon --> state_mgrs
        hook_manager --> lxd_backend
        lxd_backend --> lxd_daemon
        lxd_backend <--> img_server
        lxd_daemon --> containers
        lxd_daemon --> zfs_pool
        sdk_manager --> lxd_backend
        service_unit --> api
        socket_activation --> api
        state_mgrs --> checkpointer
        state_mgrs --> connector
        state_mgrs --> hook_manager
        state_mgrs --> sdk_manager
        state_mgrs --> state_db
        state_mgrs --> task_runner
        state_mgrs --> workshop_manager
        watchdog --> daemon
        workshop_manager --> lxd_backend
        zfs_pool --> snapshot_mgr
        zfs_pool --> volume_mgr
