.. _ref_sdk_definition:

.. meta::
   :description: Reference for the runtime sdk.yaml definition file. Covers
                 filename conventions, top-level fields, interface attributes,
                 the JSON Schema, and worked examples.

SDK definition
==============

.. @artefact SDK definition

The :file:`sdk.yaml` file is the *runtime* SDK definition:
|ws_markup| reads it when it installs an SDK in a workshop.
For Store SDKs and SDKs from :command:`sdkcraft try`,
this file is produced by |sdk_markup| from :file:`sdkcraft.yaml`
(see :ref:`ref_sdkcraft_definition`).
For sketch SDKs and in-project SDKs,
you author :file:`sdk.yaml` directly:
sketch SDKs through :command:`workshop sketch-sdk`,
in-project SDKs by hand under :file:`.workshop/`.


Filename and location
---------------------

.. @artefact SDK definition

- Store SDKs and SDKs from :command:`sdkcraft try` ship :file:`sdk.yaml`
  inside their packed contents at :file:`meta/sdk.yaml`.

- In-project SDKs use
  :file:`.workshop/<NAME>/sdk.yaml` or :file:`.workshop/<NAME>/meta/sdk.yaml`,
  relative to the project directory.
  Their hook scripts live next to the definition,
  under :file:`.workshop/<NAME>/hooks/`.

- Sketch SDK definitions live in the per-workshop data directory:
  :file:`~/.local/share/workshop/id/<PROJECT-ID>/<WORKSHOP>/sdk/sketch/current/sdk.yaml`.


In-project and sketch SDKs do not support |sdk_markup| build-time features
such as :samp:`build-base`, :samp:`platforms`, or :samp:`parts`.
These belong to :file:`sdkcraft.yaml`.


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

   * - :samp:`architecture` (required for built SDKs)
     - string
     - CPU architecture the SDK is built for,
       following `Debian's naming scheme <https://www.debian.org/ports/>`__
       (for example, :samp:`amd64`, :samp:`arm64`).
       Use :samp:`all` for SDKs that ship no compiled binaries.

   * - :samp:`version` (required for built SDKs)
     - string
     - SDK version. Semantic versioning is recommended.

       .. note::

          Quote version strings in YAML when they look numeric
          (for example, :samp:`version: "1.0"`) to avoid type coercion.

   * - :samp:`summary` (required for built SDKs)
     - string
     - One-line summary, up to 78 characters.

   * - :samp:`description` (required for built SDKs)
     - string
     - Longer free-form description, up to about a hundred words.

   * - :samp:`sdkcraft-started-at` (required for built SDKs)
     - string
     - UTC timestamp marking when |sdk_markup| started the build.
       Set automatically; do not edit by hand.

   * - :samp:`base`
     - string
     - Base operating system image the SDK targets.
       One of :samp:`ubuntu@20.04`, :samp:`ubuntu@22.04`, :samp:`ubuntu@24.04`,
       or :samp:`ubuntu@26.04`.
       Omit for SDKs that work on any supported base.

   * - :samp:`title`
     - string
     - Human-readable title.

   * - :samp:`license`
     - string
     - License name, as it would appear in package metadata.

       .. note::

          Match the license to the actual components the SDK installs.

   * - :samp:`contact` (built SDKs only)
     - string, array, or URL
     - Contact information for the SDK publisher.
       Copied from :file:`sdkcraft.yaml` by |sdk_markup|;
       |ws_markup| ignores it.

   * - :samp:`issues` (built SDKs only)
     - string, array, or URL
     - Where users should report problems with the SDK.
       Copied from :file:`sdkcraft.yaml` by |sdk_markup|;
       |ws_markup| ignores it.

   * - :samp:`source-code` (built SDKs only)
     - URL
     - Where the SDK's source code is hosted.
       Copied from :file:`sdkcraft.yaml` by |sdk_markup|;
       |ws_markup| ignores it.

   * - :samp:`website` (built SDKs only)
     - URL
     - The web page for the SDK.
       Copied from :file:`sdkcraft.yaml` by |sdk_markup|;
       |ws_markup| ignores it.

   * - :samp:`plugs`
     - object
     - Plugs the SDK requests from the workshop environment.
       Each key is the plug name; each value is an inline plug definition.
       See :ref:`ref_sdk_definition_interfaces`.

   * - :samp:`slots`
     - object
     - Slots the SDK provides.
       Each key is the slot name;
       each value is an inline slot definition.
       Only the :samp:`mount` and :samp:`tunnel` interfaces
       support slots on regular SDKs.
       See :ref:`ref_sdk_definition_interfaces`.


.. note::

   "Required for built SDKs" means |sdk_markup| writes the field
   when it builds an SDK package;
   for an in-project SDK,
   you can author :file:`sdk.yaml` with only :samp:`name`,
   plus whichever optional fields you need.
   In particular,
   :samp:`architecture` for in-project SDKs is assumed
   to match the host (or :samp:`all`).

   "Built SDKs only" means |ws_markup| accepts the field
   in a packed :file:`sdk.yaml` but does not use it.


.. _ref_sdk_definition_unknown_keys:

Unknown keys
------------

|ws_markup| reads the two kinds of :file:`sdk.yaml` differently:

- An in-project :file:`sdk.yaml` is validated strictly.
  Any top-level key outside the table above,
  including the built-SDKs-only keys
  :samp:`contact`, :samp:`issues`, :samp:`source-code`, and :samp:`website`,
  fails :command:`workshop launch`:

  .. code-block:: text

     error: cannot launch "my-workshop": invalid "ccache" SDK: unknown SDK YAML fields: website (line 2, column 10)

  The message names every unknown key with its line and column.

- A built :file:`sdk.yaml`, as shipped by Store SDKs and SDKs from :command:`sdkcraft try`,
  is read leniently:
  |ws_markup| picks the fields it understands and ignores the rest.


.. _ref_sdk_definition_interfaces:

Interfaces
----------

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

The following JSON Schema is exported from |sdk_markup|'s runtime metadata model
and describes the structure above:

.. note::

   The schema describes a *built* :file:`sdk.yaml`,
   that is, the file |sdk_markup| writes when it packs an SDK.
   The :samp:`required` list reflects what a packed SDK must carry;
   for an in-project :file:`sdk.yaml` you author by hand,
   only :samp:`name` is mandatory
   (see the note below the table).
   Because the schema describes what |sdk_markup| writes,
   it also lists the built-SDKs-only keys
   :samp:`contact`, :samp:`issues`, :samp:`source-code`, and :samp:`website`,
   which an in-project :file:`sdk.yaml` cannot contain
   (see :ref:`ref_sdk_definition_unknown_keys`).
   Rely on the field table for what an in-project file may carry.

   Numeric bounds use pydantic-style :samp:`ge`, :samp:`le`, and :samp:`lt` keywords.
   Generic JSON Schema validators will not enforce them;
   treat the bounds as documentation of the runtime's accepted ranges,
   and rely on the field table above for the authoritative rules.


.. dropdown:: SDK definition schema

   .. literalinclude:: schema-sdk.json
      :language: json


Examples
--------

In-project SDK that declares a mount plug:

.. literalinclude:: ../../examples/sdk-project-cache.yaml
   :language: yaml
   :caption: .workshop/ccache/sdk.yaml

Runtime :file:`sdk.yaml` written by |sdk_markup| for a Go development SDK:

.. literalinclude:: ../../examples/sdk-go-runtime.yaml
   :language: yaml
   :caption: meta/sdk.yaml


See also
--------

Explanation:

- :ref:`exp_in_project_sdk`
- :ref:`exp_sdk_concepts`
- :ref:`exp_sdk_definition`
- :ref:`exp_system_sdk`

Reference:

- :ref:`ref_sdk_internals`
- :ref:`ref_sdkcraft_definition`
- :ref:`ref_workshop_definition`

Tutorial:

- :ref:`tut_sketch_sdks`
