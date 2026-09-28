.. _ref_sdk_internals:

.. meta::
   :description: Reference guide to SDK internals, including source directory
                 structure, parts, plugs, slots, and modularization in Workshop SDKs.

SDK internals
=============

.. @artefact SDK

.. _ref_sdk_directory:

Source directory
----------------

All files that go into an SDK should be placed in a *source directory*
where you'll run |sdk_markup|
to initialize, define, pack and publish the SDK.


.. _ref_sdk_platform:

SDK platform
------------

.. @artefact SDK platforms

A platform describes where an SDK can be built and installed.

The components describing a platform are:

- The base image: used to build SDKs and initialize workshops.
- The CPU architecture: :samp:`amd64`, :samp:`arm64`, :samp:`armhf`, :samp:`i386`,
  :samp:`ppc64el`, :samp:`riscv64`, or :samp:`s390x`.

The easiest way to define a platform
is to name it after the CPU architecture:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   # ...
   base: ubuntu@24.04
   platforms:
     amd64:
     arm64:


The above SDK can be built on :samp:`amd64` or :samp:`arm64` machines
and installed in :samp:`ubuntu@24.04` workshops with the same architecture.

The :samp:`base` can also be moved into the platform names:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   # ...
   platforms:
     ubuntu@24.04:amd64:
     ubuntu@24.04:arm64:


More complex scenarios can be described using the following attributes:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 3 2 15

   * - Key
     - Value
     - Description

   * - :samp:`build-on`
     - array
     - List of supported CPU architectures
       that can build the SDK for the platform.

       If the SDK has no :samp:`base` or :samp:`build-base`,
       each entry must be prefixed by a valid base and a colon,
       e.g., :samp:`ubuntu@22.04:amd64`.
       This has no effect on the supported build machines,
       because |sdk_markup| performs builds in containers.
       The prefix must match :samp:`build-for`.

   * - :samp:`build-for`
     - string
     - CPU architecture the SDK is expected to run on,
       or :samp:`all` if the SDK can run on all supported architectures.
       SDK authors are responsible for ensuring compatibility.

       If the SDK has no :samp:`base` or :samp:`build-base`,
       each entry must be prefixed by a valid base and a colon,
       e.g., :samp:`ubuntu@24.04:riscv64`.
       The prefix must match every entry in :samp:`build-on`.


Architecture-independent SDKs require the complex format:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   # ...
   platforms:
     all:
       build-on: [amd64, arm64, riscv64]
       build-for: all


.. _ref_sdk_parts:

SDK parts
---------

.. @artefact sdkcraft (CLI)
.. @artefact SDK part

Parts can be thought of as the building blocks of |ws_markup| and |sdk_markup|.
Each part in the :file:`sdkcraft.yaml` :ref:`definition <ref_sdk_definition>`
describes a specific component or piece of the SDK being packaged,
providing a way to modularize the package and manage its dependencies.

|sdk_markup| is built as a
`craft-application <https://github.com/canonical/craft-application/>`_,
which affects how :samp:`parts` are implemented.
However, note that :samp:`stage-packages` and :samp:`stage-snaps`
aren't enabled yet;
instead, rely on the :ref:`hooks <ref_sdk_hooks>`
to implement custom logic of package and snap installation.

For a complete reference of :samp:`parts` and their properties,
refer to the corresponding Craft Parts
`documentation section
<https://documentation.ubuntu.com/craft-parts/latest/common/craft-parts/reference/part_properties/>`_.


.. _ref_sdk_plugs_slots:

SDK plugs and slots
-------------------

.. @artefact interface plug
.. @artefact interface slot

Currently, |ws_markup| and |sdk_markup| support the following interface plugs:

- :ref:`Camera <ref_camera_interface>`
- :ref:`Custom device <ref_custom_device_interface>`
- :ref:`Desktop <ref_desktop_interface>`
- :ref:`GPU <ref_gpu_interface>`
- :ref:`Mount <ref_mount_interface>`
- :ref:`SSH <ref_ssh_interface>`
- :ref:`Tunnel <ref_tunnel_interface>`


Regular SDKs can define slots only for the :samp:`mount` and :samp:`tunnel` interfaces.
Slots for the other interfaces are built into the :samp:`system` SDK
and cannot be added to it under another name.

.. _ref_camera_interface:

Camera interface
~~~~~~~~~~~~~~~~

.. @artefact camera interface

A camera plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdk.yaml

    # ...
    plugs:
      <NAME>:
        interface: camera


This makes the host's cameras directly available inside the workshop
as video capture devices.


.. _ref_custom_device_interface:

Custom device interface
~~~~~~~~~~~~~~~~~~~~~~~

.. @artefact custom-device interface

A custom-device plug in the definition
must specify the plug name, the interface,
and a :samp:`subsystem` attribute:

.. code-block:: yaml
   :caption: sdk.yaml

    # ...
    plugs:
      <NAME>:
        interface: custom-device
        subsystem: <SUBSYSTEM>


This makes host devices from the given subsystem directly available inside the workshop.


.. _ref_desktop_interface:

Desktop interface
~~~~~~~~~~~~~~~~~

.. @artefact desktop interface

A desktop plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdk.yaml

    # ...
    plugs:
      <NAME>:
        interface: desktop


This makes the host's Wayland socket directly available inside the workshop.


.. _ref_gpu_interface:

GPU interface
~~~~~~~~~~~~~

.. @artefact GPU interface

A GPU plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdk.yaml

    # ...
    plugs:
      gpu:
        interface: gpu


This makes the host's GPUs directly available inside the workshop
via the GPU pass-through mechanism.


.. _ref_mount_interface:

Mount interface
~~~~~~~~~~~~~~~

.. @artefact mount interface

A mount plug in the definition must specify the plug name, the interface, and the target directory.
The plug can specify permissions and ownership for the target, and whether it is read-only:

.. code-block:: yaml
   :caption: sdk.yaml

    # ...
    plugs:
      <NAME>:
        interface: mount
        workshop-target: <WORKSHOP DIRECTORY>
        mode: <OCTAL FILE MODE> # optional
        uid: <USER ID> # optional
        gid: <GROUP ID> # optional
        read-only: <true | false> # optional

.. @artefact $SDK

This mounts a directory automatically created by |ws_markup| on the host
to the :samp:`workshop-target` directory.
The :envvar:`$SDK` variable can be used to refer to the SDK installation path
inside the workshop.
The host directory will be created under the path
designated by the :envvar:`$XDG_DATA_HOME` variable.
The workshop directory will be created using the given :samp:`mode`, :samp:`uid`, and :samp:`gid`.

A mount *slot* in the definition must specify the slot name, the interface,
and the *source* directory:

.. code-block:: yaml
   :caption: sdk.yaml

    # ...
    slots:
      <NAME>:
        interface: mount
        workshop-source: <WORKSHOP DIRECTORY>

.. @artefact $SDK

This exposes the :samp:`workshop-source` directory inside the workshop
to be mounted to another directory within the workshop.
The :envvar:`$SDK` variable can be used to refer to the SDK installation path
inside the workshop.


.. _ref_ssh_interface:

SSH interface
~~~~~~~~~~~~~

.. @artefact SSH interface

An SSH plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdk.yaml

    # ...
    plugs:
      ssh-agent:
        interface: ssh-agent


This proxies the host's SSH keys and configuration inside the workshop
via a Unix domain socket.


.. _ref_tunnel_interface:

Tunnel interface
~~~~~~~~~~~~~~~~

.. @artefact tunnel interface

A tunnel plug in the definition must specify the plug name, the interface and optionally an endpoint:

.. code-block:: yaml
   :caption: sdk.yaml

   # ...
   plugs:
     <NAME>:
       interface: tunnel
       endpoint: <ENDPOINT>


Similarly, a tunnel *slot* in the definition must specify the slot name, the interface and optionally an endpoint:

.. code-block:: yaml
   :caption: sdk.yaml

   # ...
   slots:
     <NAME>:
       interface: tunnel
       endpoint: <ENDPOINT>


When a tunnel interface plug is connected to a slot,
clients can connect to the address of the plug.
The connection will be forwarded to the address of the slot.
Regular SDKs define the workshop side of the connection,
leaving the host system to the :samp:`system` SDK.

The supported protocols are TCP, UDP and Unix domain sockets.
Unix domain sockets are compatible with TCP, but UDP plugs can only connect to UDP slots.

TCP and UDP endpoints look like :samp:`<IPv4>:<PORT>/<PROTOCOL>` or :samp:`'[<IPv6>]:<PORT>/<PROTOCOL>'`.
These endpoints don't resolve hostnames,
including workshop :samp:`*.wp` hostnames,
but support the aliases :samp:`localhost`, :samp:`ip6-localhost` and :samp:`ip6-loopback`.

Unix domain socket endpoints are either paths to a socket file or abstract sockets of the form :samp:`'@<STRING>'`.
The :envvar:`$HOME` and :envvar:`$XDG_RUNTIME_DIR` variables can be used in paths.

Attributes can be abbreviated by omitting :samp:`tcp` and :samp:`localhost`:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 4 10

   * - Address
     - Alternatives

   * - :samp:`127.0.0.1:1234/tcp`

     - :samp:`localhost:1234/tcp`, :samp:`1234/tcp`, :samp:`127.0.0.1:1234`, :samp:`1234`

   * - :samp:`0.0.0.0:1234/tcp`

     - :samp:`0.0.0.0:1234`

   * - :samp:`'[::1]:1234/tcp'`

     - :samp:`ip6-localhost:1234/tcp`, :samp:`ip6-loopback:1234`, :samp:`'[::1]:1234'`

   * - :samp:`127.0.0.1:1234/udp`

     - :samp:`localhost:1234/udp`, :samp:`1234/udp`

   * - :samp:`'[::]:1234/udp'`

     -

   * - :samp:`/run/service.sock`

     -

   * - :samp:`'@abstract'`

     -


Port numbers may also be omitted,
but only on one side of a connection.
For such connections,
both sides use the same port.


.. _ref_sdk_hooks:

SDK hooks
---------

|ws_markup| supports the following lifecycle hooks,
which can be defined when the SDK is built using |sdk_markup|:

.. @artefact workshopctl
.. @artefact check-health
.. @artefact workshop status
.. @artefact restore-state
.. @artefact save-state
.. @artefact SDK base image
.. @artefact setup-base
.. @artefact workshop base image
.. @artefact setup-project

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 3 6 5

   * - Name
     - When |ws_markup| runs it
     - What it does

   * - :samp:`setup-base`

     - At :ref:`ref_workshop_launch`, :ref:`ref_workshop_refresh`:
       after unpacking the base image
       and starting the workshop,
       but before mounting the project directory
       and connecting plugs and slots.

     - Configures system packages and services required by the SDK.

   * - :samp:`setup-project`

     - At :ref:`ref_workshop_launch`, :ref:`ref_workshop_refresh`, :ref:`ref_workshop_restore`:
       after mounting the project directory
       and auto-connecting plugs and slots
       but before the workshop is set to *Ready*.

     - Configures the user environment for the SDK to become operational.

   * - :samp:`save-state`

     - At :ref:`ref_workshop_refresh`, :ref:`ref_workshop_restore`:
       before destroying the old workshop.

     - Saves SDK-specific data to the :ref:`state directory <ref_sdk_state>`.
       The hook itself comes from the *old* SDK revision.

   * - :samp:`restore-state`

     - At :ref:`ref_workshop_refresh`, :ref:`ref_workshop_restore`:
       after running :samp:`setup-project` hooks for *all* SDKs.

     - Restores SDK-specific data from the :ref:`state directory <ref_sdk_state>`.
       The hook itself comes from the *new* SDK revision.

   * - :samp:`check-health`
     - At :ref:`ref_workshop_launch`:
       after running :samp:`setup-project` hooks for *all* SDKs.

       At :ref:`ref_workshop_refresh`, :ref:`ref_workshop_restore`:
       after running :samp:`restore-state` hooks for *all* SDKs.

     - Sets the state of the SDK
       (:samp:`okay`, :samp:`waiting` or :samp:`error`)
       using :ref:`workshopctl <ref_workshopctl__cli>`,
       which affects the :ref:`status <ref_workshop_status>` of the workshop.


Each hook is defined as a :program:`bash` script of the same name
under :samp:`hooks/` in the :ref:`source directory <ref_sdk_directory>`.
Inside the workshop,
the SDK is mounted at :file:`/var/lib/workshop/sdk/<SDK>/`
and hooks are stored in the :file:`sdk/hooks/` subdirectory.
Most hooks run as :samp:`root`
and use that subdirectory as the working directory.
The exception is :samp:`setup-project`,
which runs as the :samp:`workshop` user
in the :file:`/project/` directory.

A hook can signal an error by returning a nonzero exit code;
a zero code indicates success.
The options :samp:`errexit` and :samp:`pipefail`
are set by default,
so most commands which return a nonzero exit code
cause the hook to exit with the same code.
If :option:`!--verbose` is passed to :command:`workshop launch` or :command:`workshop refresh`,
the option :samp:`xtrace` is also set.

.. note::

   The hooks aren't mentioned in the :ref:`SDK definition <ref_sdk_definition>`;
   |sdk_markup| automatically enumerates them when packing the SDK.

   An SDK's position in the :ref:`workshop definition <ref_workshop_definition>`
   determines when its hooks execute.
   SDKs are always processed in the following order:
   :samp:`system`, user-listed SDKs, :samp:`sketch`.
   Each hook waits for the previous one to complete before executing.


.. _ref_sdk_state:

SDK state
---------

.. @artefact SDK state

An SDK can store any data specific to it within the workshop.
For this purpose, an environment variable named :envvar:`$SDK_STATE_DIR`
is exposed by |ws_markup| at runtime;
it resolves to an internal directory in the workshop,
which :samp:`save-state` and :samp:`restore-state`
can use to preserve and recover the data respectively.

.. note::

  The :envvar:`$SDK_STATE_DIR` variable is only available
  to the :samp:`save-state` and :samp:`restore-state` SDK hooks.
  It is not accessible to the :samp:`workshop` user, the SDK itself,
  or in the workshop definition.

  The state directory is a dedicated volume created by |ws_markup| at runtime
  for each SDK in every workshop,
  and is removed when the workshop stops.
  The :samp:`*-state` hooks can use it
  to store or retrieve any arbitrary data required by the SDK.


.. _ref_sdk_channels:

SDK channels
------------

.. @artefact SDK channel

When SDKs are published by their creators and consumed by workshops,
different versions and releases are tracked through the use of channels.
A channel is a combination of a track, a risk, and an optional branch,
e.g., :samp:`latest/beta`.

Tracks allow multiple published versions of an SDK to exist in parallel;
while no specific scheme is enforced,
it is desirable to use a semantic version, e.g., :samp:`1.2.3`,
or the :samp:`latest` keyword,
which maps to the latest published version and serves as the default.

Risks represent a choice of maturity levels for a particular track:

- :samp:`stable`: indicates that the software can be used in production

- :samp:`candidate`: for software that's being tested prior to stable deployment

- :samp:`beta`: for software that can be used outside of production

- :samp:`edge`: for unstable software that's still in active development;
  nothing is guaranteed

Branches are short-lived subdivisions of a channel
intended for experimentation, e.g. `1.2.3/edge/issue-56789`.
After 30 days of no activity, a branch will be closed automatically.

.. note::

   SDK channels should not be confused with SDK revisions.
   Also, a channel is independent of a platform.
   The same channel resolves to a separate revision
   for each :ref:`platform <ref_sdk_platform>` the SDK supports,
   so workshops with different bases or architectures
   receive different builds under one channel name.


See also
--------

Explanation:

- :ref:`exp_base`
- :ref:`exp_camera_interface`
- :ref:`exp_custom_device_interface`
- :ref:`exp_desktop_interface`
- :ref:`exp_gpu_interface`
- :ref:`exp_interface_concepts`
- :ref:`exp_mount_interface`
- :ref:`exp_sdks`
- :ref:`exp_ssh_interface`
- :ref:`exp_system_sdk`
- :ref:`exp_tunnel_interface`
- :ref:`exp_workshop_definition`
