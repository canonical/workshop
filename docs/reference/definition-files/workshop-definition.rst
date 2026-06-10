.. _ref_workshop_definition:

.. meta::
   :description: Reference for the workshop.yaml definition file. Covers
                 filename conventions, top-level fields, nested structures,
                 interface attributes, the JSON Schema, and worked examples.

Workshop definition
===================

.. @artefact project
.. @artefact workshop definition

A *workshop definition* is the YAML file
that |ws_markup| reads to launch and refresh a workshop.
It names the base image, lists the SDKs to install,
declares any extra plugs, slots, or connections,
and records reusable shell actions.
The file is authored by the workshop's user.


Filename and location
---------------------

.. @artefact project workshops
.. @artefact workshop name

A project may store a single workshop definition at its root,
or several under :file:`.workshop/`.

- A single workshop: :file:`workshop.yaml` or :file:`.workshop.yaml`
  in the project directory.

- Multiple workshops: :file:`.workshop/<NAME>.yaml`, one file per workshop.
  The :samp:`<NAME>` part of the filename
  must equal the workshop's :samp:`name` field.

- A workshop name must start with a lowercase letter
  and may contain lowercase letters, digits, and hyphens between them.
  Up to 40 characters.


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
     - Workshop identifier. Subject to the naming rules above.
       Must match the filename when the definition is under :file:`.workshop/`.

   * - :samp:`base` (required)
     - string
     - Base operating system image.
       One of :samp:`ubuntu@20.04`, :samp:`ubuntu@22.04`, :samp:`ubuntu@24.04`,
       or :samp:`ubuntu@26.04`.

       SDKs that declare a :samp:`base` must use the same value;
       SDKs without a :samp:`base` are accepted on any workshop.

   * - :samp:`sdks`
     - array
     - Ordered list of SDK entries.
       Each entry references an existing SDK and configures it for the workshop.
       The system SDK is installed first implicitly and is not required here.
       See :ref:`ref_workshop_definition_sdk_entry`.

   * - :samp:`connections`
     - array
     - Explicit connections between plugs and slots,
       applied on top of |ws_markup|'s auto-connect logic.
       See :ref:`ref_workshop_definition_connection_entry`.

   * - :samp:`actions`
     - object
     - Named shell scripts available via :command:`workshop run`.
       See :ref:`ref_workshop_definition_action_entry`.


Nested structures
-----------------

.. _ref_workshop_definition_sdk_entry:

SDK entry
~~~~~~~~~

.. @artefact plug binding
.. @artefact $SDK

Each item in :samp:`sdks` is an object with these fields:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`name` (required)
     - string
     - SDK identifier. The underlying name must contain
       at least one lowercase letter and may consist of
       lowercase letters, digits, and hyphens between them.
       :samp:`agent` is reserved.

       Use a prefix to select the source:

       - no prefix: an SDK from the SDK Store (default).
       - :samp:`try-<NAME>`:
         a locally tried SDK in the :ref:`try area <ref_sdkcraft_try>`.

       - :samp:`project-<NAME>`:
         an in-project SDK defined under :file:`.workshop/<NAME>/`.

       - :samp:`system`:
         the built-in system SDK; listing it explicitly is rarely needed.

       The fully prefixed name is at most 40 characters without a prefix,
       44 with :samp:`try-`, and 48 with :samp:`project-`.

   * - :samp:`channel`
     - string
     - Store channel from which to retrieve the SDK
       at :ref:`launch <ref_workshop_launch>`
       and :ref:`refresh <ref_workshop_refresh>`.
       Uses the `snap channel format <https://snapcraft.io/docs/channels/>`__:
       :samp:`<TRACK>/<RISK>/<BRANCH>`,
       with all three parts optional except that at least one must be present.

       Default: :samp:`latest/stable`.
       Has no effect for :samp:`try-`, :samp:`project-`, and :samp:`system` SDKs,
       but must still be well formed.

       .. note::

          Quote channel values in YAML when they look numeric
          (for example, :samp:`channel: "1.26"`)
          to avoid type coercion.

   * - :samp:`plugs`
     - object
     - Plug bindings or additional plug definitions grafted onto the SDK
       by this workshop.
       See :ref:`ref_workshop_definition_plug_entry`
       and :ref:`ref_workshop_definition_interfaces`.

   * - :samp:`slots`
     - object
     - Additional slot definitions grafted onto the SDK by this workshop.
       Each entry specifies the :samp:`interface`
       and any interface-specific attributes.
       See :ref:`ref_workshop_definition_interfaces`.


.. _ref_workshop_definition_plug_entry:

Plug entry (under an SDK)
~~~~~~~~~~~~~~~~~~~~~~~~~

Each plug under an SDK is either an inline plug definition
or a binding to another plug.

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`interface`
     - string
     - Required for an inline plug definition; identifies the interface
       (for example, :samp:`mount`, :samp:`tunnel`).
       See :ref:`ref_workshop_definition_interfaces`
       for the attributes each interface accepts.

   * - :samp:`bind`
     - string
     - Reference to a target plug, in the form :samp:`<SDK>:<PLUG>`.
       The :samp:`<SDK>` part must name a non-system SDK,
       since bound plugs cannot target the system SDK.

       A bound plug must not carry any other attributes,
       cannot belong to the system SDK, cannot chain
       (bind to a plug that is itself bound),
       and cannot also appear in :samp:`connections`.

   * - any interface attribute
     - varies
     - Inline plug definitions accept the attributes
       documented under :ref:`ref_workshop_definition_interfaces`.


.. _ref_workshop_definition_connection_entry:

Connection entry
~~~~~~~~~~~~~~~~

Each item in :samp:`connections` links a plug to a slot of the same interface:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`plug` (required)
     - string
     - Plug reference, in the form :samp:`<SDK>:<PLUG>`.
       The :samp:`<SDK>` part may be empty (for example, :samp:`:ssh-agent`)
       to refer to the system SDK.
       The referenced SDK must appear in :samp:`sdks` or be implicit
       (:samp:`system`, :samp:`sketch`).

   * - :samp:`slot` (required)
     - string
     - Slot reference, in the form :samp:`<SDK>:<SLOT>`.
       Same rules as :samp:`plug`.


A plug that has a :samp:`bind` set under its SDK entry
cannot also be listed in :samp:`connections`.


.. _ref_workshop_definition_action_entry:

Action entry
~~~~~~~~~~~~

.. @artefact workshop actions

Each entry in :samp:`actions` maps an action name to a shell script body:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - action name
     - string
     - Must start with a lowercase letter and may contain lowercase letters,
       digits, and hyphens between them.

   * - action body
     - string
     - A :program:`bash` script.
       |ws_markup| sets :samp:`errexit` and :samp:`pipefail` before running it.
       Arguments passed after :command:`workshop run <WORKSHOP>` are available
       as the standard positional parameters :samp:`"$@"`, :samp:`"$1"`,
       and so on.


Actions are interpreted lazily:
edits to :samp:`actions` are available immediately,
without :command:`workshop refresh`.


.. _ref_workshop_definition_interfaces:

Interfaces
----------

The attributes accepted by inline plug and slot definitions
depend on the interface.
These same attributes appear in SDK definitions
(:ref:`ref_sdk_definition` and :ref:`ref_sdkcraft_definition`);
a workshop may graft additional plugs and slots that follow them.

.. include:: _interfaces/camera.rst

.. include:: _interfaces/custom-device.rst

.. include:: _interfaces/desktop.rst

.. include:: _interfaces/gpu.rst

.. include:: _interfaces/mount.rst

.. include:: _interfaces/ssh-agent.rst

.. include:: _interfaces/tunnel.rst


JSON Schema
-----------

.. @artefact workshop schema

The following JSON Schema describes the structure above:

.. dropdown:: Workshop definition schema

   .. literalinclude:: schema.json
      :language: json


Examples
--------

Minimal workshop with one Store SDK and two actions:

.. literalinclude:: ../../examples/workshop-golang.yaml
   :language: yaml
   :caption: .workshop/golang.yaml


Workshop with an in-project SDK and a plug binding between SDKs:

.. literalinclude:: ../../examples/workshop-go-dev.yaml
   :language: yaml
   :caption: .workshop/go-dev.yaml

Workshop that grafts a plug and a slot onto its SDKs
and adds explicit connections;
besides using the fictional
:samp:`tensorflow`, :samp:`imagenet` and :samp:`cuda` SDKs,
it defines an additional slot under the :samp:`imagenet` SDK,
a plug under :samp:`tensorflow`,
and two connections:

- One that connects the :samp:`tensorflow:images` plug
  to the newly defined :samp:`imagenet:images` slot.

- Another that connects the :samp:`tensorflow:cuda` plug
  to the preexisting :samp:`cuda:libs`.

.. literalinclude:: ../../examples/workshop-digits-cuda.yaml
   :language: yaml
   :caption: .workshop/digits-cuda.yaml


Workshop that pulls an SDK from the try area:

.. literalinclude:: ../../examples/workshop-try-go.yaml
   :language: yaml
   :caption: .workshop/try-go.yaml


See also
--------

Explanation:

- :ref:`exp_base`
- :ref:`exp_in_project_sdk`
- :ref:`exp_sdks`
- :ref:`exp_system_sdk`
- :ref:`exp_test_try_sdk`
- :ref:`exp_workshop_definition`

Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_sdkcraft_definition`
- :ref:`ref_workshop_info`
