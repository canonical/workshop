.. _ref_sdkcraft_definition:

.. meta::
   :description: Reference for the sdkcraft.yaml build-time definition file.
                 Covers filename conventions, top-level fields, platforms, parts,
                 interface attributes, the JSON Schema, and worked examples.

SDKcraft project definition
===========================

.. @artefact sdkcraft (CLI)
.. @artefact SDK
.. @artefact SDK definition

The :file:`sdkcraft.yaml` file is the *build-time* SDK definition:
|sdk_markup| reads it to pack an SDK. SDK publishers author this file;
|sdk_markup| writes the runtime :file:`sdk.yaml` (see :ref:`ref_sdk_definition`)
into the resulting package, copying plug, slot, and metadata fields across.

|sdk_markup| builds on the
`craft-application <https://canonical-craft-application.readthedocs-hosted.com/>`__
framework and
`craft-parts <https://canonical-craft-parts.readthedocs-hosted.com/>`__
for build orchestration. Many fields are inherited from :samp:`craft-application`.


Filename and location
---------------------

.. @artefact SDK definition file

- The definition file is :file:`sdkcraft.yaml` or :file:`.sdkcraft.yaml`
  at the project root.
- Hooks live next to it under :file:`hooks/`;
  |sdk_markup| lints them with `ShellCheck <https://www.shellcheck.net/>`__
  and packs them with the SDK.


Top-level fields
----------------

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`name` (required)
     - string
     - SDK identifier. Must contain at least one lowercase letter
       and may consist of lowercase letters, digits, and hyphens between them.
       Up to 40 characters.
       Cannot be :samp:`agent`, :samp:`system`, :samp:`sketch`,
       or start with :samp:`try-` or :samp:`project-`; those names are reserved.

   * - :samp:`platforms` (required)
     - object
     - Platforms the SDK can be built on and for.
       See :ref:`ref_sdkcraft_definition_platforms`.

   * - :samp:`base`
     - string
     - Base operating system image the SDK targets at runtime.
       One of :samp:`ubuntu@20.04`, :samp:`ubuntu@22.04`, :samp:`ubuntu@24.04`,
       or :samp:`ubuntu@26.04`.
       Omit for SDKs that work on any supported base.

   * - :samp:`build-base`
     - string
     - Base operating system image used to build the SDK.
       Required when :samp:`base` is omitted.

   * - :samp:`version`
     - string
     - SDK version. Semantic versioning is recommended.

       .. note::

          Quote version strings in YAML when they look numeric
          (for example, :samp:`version: "1.0"`).

   * - :samp:`title`
     - string
     - Human-readable title.

   * - :samp:`summary`
     - string
     - One-line summary, up to 78 characters.

   * - :samp:`description`
     - string
     - Longer free-form description, up to about a hundred words.

   * - :samp:`license`
     - string
     - License name, as it would appear in package metadata.
       Match the license to the actual components the SDK installs.

   * - :samp:`contact`
     - string, array, or URL
     - Contact information for the SDK publisher.

   * - :samp:`issues`
     - string, array, or URL
     - Where users should report problems with the SDK.

   * - :samp:`source-code`
     - URL
     - Where the SDK's source code is hosted.

   * - :samp:`adopt-info`
     - string
     - Name of a part whose :samp:`craftctl set` commands provide
       :samp:`version` or :samp:`summary` at build time.
       Standard :samp:`craft-application` machinery.

   * - :samp:`package-repositories`
     - array
     - Additional package repositories to enable while building.
       Standard :samp:`craft-application` machinery;
       see the `craft-archives reference <https://canonical-craft-archives.readthedocs-hosted.com/>`__.

   * - :samp:`parts`
     - object
     - Build instructions, in craft-parts format.
       See :ref:`ref_sdkcraft_definition_parts`.

   * - :samp:`plugs`
     - object
     - Plugs the SDK requests from the workshop environment.
       See :ref:`ref_sdkcraft_definition_interfaces`.

   * - :samp:`slots`
     - object
     - Slots the SDK provides.
       Only the :samp:`mount` and :samp:`tunnel` interfaces support slots here.
       See :ref:`ref_sdkcraft_definition_interfaces`.


|sdk_markup| writes :samp:`name`, :samp:`base`, :samp:`version`, :samp:`title`,
:samp:`summary`, :samp:`description`, :samp:`license`, :samp:`contact`,
:samp:`issues`, :samp:`source-code`, :samp:`plugs`, and :samp:`slots`
straight into the runtime :file:`sdk.yaml`.
The other top-level fields control the build only.


Nested structures
-----------------

.. _ref_sdkcraft_definition_platforms:

Platform entry
~~~~~~~~~~~~~~

.. @artefact SDK platforms

Each entry under :samp:`platforms` declares one build target.
The key is the platform name; the value is an object:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`build-on` (required)
     - string or array of strings
     - Architectures or :samp:`<BASE>:<ARCH>` triples
       on which |sdk_markup| may build this platform.
       Entries are unique; at least one is required.

   * - :samp:`build-for` (required)
     - string or array of strings
     - Architectures or :samp:`<BASE>:<ARCH>` triples this build targets.
       Use :samp:`all` for SDKs that ship no compiled binaries.


The platform name may be shorthand
for both :samp:`build-on` and :samp:`build-for`
(for example, a key of :samp:`amd64` with no nested value).


.. _ref_sdkcraft_definition_parts:

Part entry
~~~~~~~~~~

Each entry under :samp:`parts` is a craft-parts definition:
a key naming the part, with a value that specifies a :samp:`plugin`
and the plugin's parameters.
|sdk_markup| forbids :samp:`stage-packages` and :samp:`stage-snaps` in parts;
install packages and snaps from the :samp:`setup-base` hook instead.

When :samp:`parts` is omitted,
|sdk_markup| supplies a default part
equivalent to :samp:`{default-part: {plugin: nil}}`.

For the full set of plugin types, lifecycle steps, and override mechanisms,
see the `craft-parts reference
<https://documentation.ubuntu.com/craft-parts/latest/reference/parts_steps/>`__.


.. _ref_sdkcraft_definition_interfaces:

Interfaces
----------

Plug and slot values in :file:`sdkcraft.yaml`
use the same shape as in :file:`sdk.yaml`.
A plug or slot value is an inline definition:
a mapping that specifies the :samp:`interface`
and any interface-specific attributes.

.. include:: _interfaces/camera.rst

.. include:: _interfaces/custom-device.rst

.. include:: _interfaces/desktop.rst

.. include:: _interfaces/gpu.rst

.. include:: _interfaces/mount.rst

.. include:: _interfaces/ssh-agent.rst

.. include:: _interfaces/tunnel.rst


JSON Schema
-----------

.. @artefact SDK schema

The following JSON Schema is exported from |sdk_markup|'s project model
and describes the structure above:

.. note::

   Numeric bounds use pydantic-style :samp:`ge`, :samp:`le`, and :samp:`lt` keywords.
   Generic JSON Schema validators will not enforce them;
   treat the bounds as documentation of the accepted ranges,
   and rely on the field table above for the authoritative rules.


.. dropdown:: SDKcraft project schema

   .. literalinclude:: schema-sdkcraft.json
      :language: json


Examples
--------

Complex SDK that uses :samp:`platforms`, :samp:`parts`, and a mix of plugs:

.. literalinclude:: ../../examples/go-sdkcraft.yaml
   :language: yaml
   :caption: sdkcraft.yaml

Multi-base SDK with no parts:

.. literalinclude:: ../../examples/sdkcraft-multibase.yaml
   :language: yaml
   :caption: sdkcraft.yaml

SDK that exposes mount and GPU plugs:

.. literalinclude:: ../../examples/sdkcraft-ros2.yaml
   :language: yaml
   :caption: sdkcraft.yaml


See also
--------

Explanation:

- :ref:`exp_in_project_sdk`
- :ref:`exp_sdk_concepts`
- :ref:`exp_sdk_definition`
- :ref:`exp_sdk_hooks`
- :ref:`exp_system_sdk`

Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_workshop_definition`

Tutorial:

- :ref:`tut_craft_sdks`
