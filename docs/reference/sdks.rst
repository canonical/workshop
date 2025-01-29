.. _ref_sdk:

SDKs
====

.. @artefact SDK

.. _ref_sdk_directory:

Source directory
----------------

All files that go into an SDK should be placed in a *source directory*
where you'll run |sdk_markup|
to initialise, define, pack and publish the SDK.


.. _ref_sdk_parts:

SDK parts
---------

.. @artefact sdkcraft (CLI)
.. @artefact SDK part

Parts can be thought of as the building blocks of |ws_markup| and |sdk_markup|.
Each part in the :file:`sdkcraft.yaml` :ref:`definition <ref_sdk_definition>`
describes a specific component or piece of the SDK being packaged,
providing a way to modularise the package and manage its dependencies.

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
<https://canonical-craft-parts.readthedocs-hosted.com/en/latest/common/craft-parts/reference/part_properties.html>`_.


.. _ref_sdk_plugs_slots:

SDK plugs and slots
-------------------

.. @artefact interface plug
.. @artefact interface slot

Currently, |ws_markup| and |sdk_markup| support the following interface plugs:

- :ref:`Camera <ref_camera_interface>`
- :ref:`Desktop <ref_desktop_interface>`
- :ref:`GPU <ref_gpu_interface>`
- :ref:`Mount <ref_mount_interface>`
- :ref:`SSH <ref_ssh_interface>`


Slots can only be defined for the :samp:`mount` interface.

.. _ref_camera_interface:

Camera interface
~~~~~~~~~~~~~~~~

.. @artefact camera interface

A camera plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdkcraft.yaml

    # ...
    plugs:
      <NAME>:
        interface: camera


This makes the host's USB-based cameras directly available inside the workshop
as video capture devices.

.. note::

   See the :ref:`explanation <exp_camera_interface>` for more details.


.. _ref_desktop_interface:

Desktop interface
~~~~~~~~~~~~~~~~~

.. @artefact desktop interface

A desktop plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdkcraft.yaml

    # ...
    plugs:
      <NAME>:
        interface: desktop


This makes the host's Wayland socket directly available inside the workshop.

.. note::

   See the :ref:`explanation <exp_desktop_interface>` for more details.


.. _ref_gpu_interface:

GPU interface
~~~~~~~~~~~~~

.. @artefact GPU interface

A GPU plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdkcraft.yaml

    # ...
    plugs:
      gpu:
        interface: gpu


This makes the host's GPUs directly available inside the workshop
via the GPU pass-through mechanism.

.. note::

   See the :ref:`explanation <exp_gpu_interface>` for more details.


.. _ref_mount_interface:

Mount interface
~~~~~~~~~~~~~~~

.. @artefact mount interface

A mount plug in the definition must specify the plug name, the interface, the target directory and optionally whether to be read-only:

.. code-block:: yaml
   :caption: sdkcraft.yaml

    # ...
    plugs:
      <NAME>:
        interface: mount
        workshop-target: <WORKSHOP DIRECTORY>
        read-only: <true | false> # optional


This mounts a directory automatically created by |ws_markup| on the host
to the :samp:`workshop-target` directory.
The host directory will be created under the path
designated by the :envvar:`$XDG_DATA_HOME` variable.

A mount *slot* in the definition must specify the slot name, the interface,
and the *source* directory:

.. code-block:: yaml
   :caption: sdkcraft.yaml

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

.. note::

   See the :ref:`explanation <exp_mount_interface>` for more details.


.. _ref_ssh_interface:

SSH interface
~~~~~~~~~~~~~

.. @artefact SSH interface

An SSH plug in the definition must specify the plug name and the interface:

.. code-block:: yaml
   :caption: sdkcraft.yaml

    # ...
    plugs:
      ssh-agent:
        interface: ssh-agent


This proxies the host's SSH keys and configuration inside the workshop
via a Unix domain socket.

.. note::

   See the :ref:`explanation <exp_ssh_interface>` for more details.


.. _ref_sdk_hooks:

SDK hooks
---------

|ws_markup| supports the following life cycle hooks,
which can be defined when the SDK's is built using |sdk_markup|:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 3 6 5

   * - Name
     - When |ws_markup| runs it
     - What it does

   * - :samp:`check-health`
     - At :ref:`ref_workshop_launch`:
       after running :samp:`setup-base` hooks for *all* SDKs.

       At :ref:`ref_workshop_refresh`:
       after running :samp:`restore-state` hooks for *all* SDKs.

     - Sets the state of the SDK
       (:samp:`okay`, :samp:`waiting` or :samp:`error`)
       using :ref:`workshopctl <ref_workshopctl>`,
       which affects the :ref:`status <ref_workshop_status>` of the workshop.

.. @artefact workshopctl
.. @artefact check-health
.. @artefact workshop status

   * - :samp:`restore-state`

     - At :ref:`ref_workshop_refresh`:
       after launching the new workshop
       and running the :samp:`setup-base` hook for the SDK.

     - Restores SDK-specific data from the :ref:`state directory <ref_sdk_state>`.
       The hook itself comes from the *new* SDK version.

.. @artefact restore-state

   * - :samp:`save-state`

     - At :ref:`ref_workshop_refresh`:
       before destroying the old workshop.

     - Saves SDK-specific data to the :ref:`state directory <ref_sdk_state>`.
       The hook itself comes from the *old* SDK version.

.. @artefact save-state

   * - :samp:`setup-base`

     - At :ref:`ref_workshop_launch`, :ref:`ref_workshop_refresh`:
       after unpacking the base image
       and starting the workshop,
       but before setting its status to *Ready*.

.. @artefact SDK base image
.. @artefact setup-base
.. @artefact workshop base image

     - Configures the base image for the SDK to become operational.


Each hooks is defined in a text file of the same name
under :samp:`hooks/` in the :ref:`source directory <ref_sdk_directory>`.
At run-time, they are executed as shell scripts
under :samp:`root` inside the workshop,
so each hook must start with a shebang directive,
for example:

.. code-block:: shell

   #!/usr/bin/bash


A hook can signal an error by returning a non-zero exit code;
a zero code indicates success.

.. note::

   The hooks aren't mentioned in the :ref:`definition <ref_sdk_definition>`;
   |sdk_markup| automatically enumerates them when packing the SDK.

   Hooks of the same type from multiple SDKs run non-deterministically,
   so you shouldn't rely on any particular order of their execution.


.. _ref_sdk_state:

SDK state
---------

.. @artefact SDK state

An SDK cat store any data specific to it within the workshop.
For this purpose, an environment variable named :envvar:`$SDK_STATE_DIR`
is exposed by |ws_markup| at run-time;
it resolves to an internal directory in the workshop,
which :samp:`save-state` and :samp:`restore-state`
can use to preserve and recover the data respectively.


.. _ref_sdk_channels:

SDK channels
------------

.. @artefact SDK channel

When SDKs are published by their creators and consumed by workshops,
different versions and releases are tracked through the use of channels.
A channel is a combination of a track and a risk, e.g. :samp:`latest/beta`.

Tracks allow multiple published versions of an SDK to exist in parallel;
while no specific scheme is enforced,
it is desirable to use a semantic version, e.g. :samp:`1.2.3`,
or the :samp:`latest` keyword,
which maps to the latest published version and serves as the default.

Risks represent a choice of maturity levels for a particular track:

- :samp:`stable`: indicates that the software can be used in production

- :samp:`candidate`: for software that's being tested prior to stable deployment

- :samp:`beta`: for software that can be used outside of production

- :samp:`edge`: for unstable software that's still in active development;
  nothing is guaranteed


See also
--------

Explanation:

- :ref:`exp_base`
- :ref:`exp_interfaces`
- :ref:`exp_sdk`
- :ref:`exp_sdk_state`
- :ref:`exp_workshop_definition`
