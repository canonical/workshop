.. _ref_workshop_def:

Workshop definition
===================

Filename convention
-------------------

The path of the workshop definition
file must have the following format: :file:`workshop.<NAME>.yaml`.

.. tip:: Workshop definitions should be located in the :file:`.workshop` folder.


Here, :samp:`<NAME>` is a placeholder that stands for the actual name
of the workshop itself;
it must start with a lowercase letter
and may include only lowercase letters, digits, hyphens or underscores.


Description
-----------

The definition in the file is written in `YAML <https://yaml.org/>`__
and includes a number of mandatory and optional keys:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 1 1 7

   * - Key
     - Value
     - Description

   * - :samp:`name` (required)
     - string
     - Workshop's name, used to reference the workshop itself.

       Must be the same as :samp:`<NAME>`
       in the workshop definition's filename.

   * - :samp:`base` (required)
     - string
     - Workshop's base image
       that provides the underlying OS capabilities.

       It can be :samp:`ubuntu@20.04`, :samp:`ubuntu@22.04`
       or :samp:`ubuntu@24.04`.

   * - :samp:`sdks`
     - object
     - List of individual SDKs
       from the SDK Store to include in the workshop.

       Each entry points to an existing SDK
       and specifies its retrieval channel.

   * - :samp:`connections`
     - array
     - List of connections made by the workshop;
       each links a plug to a slot.

       Any entry in :samp:`connections` must include a :samp:`plug` and a
       :samp:`slot` from the SDKs listed under :samp:`sdks` (the system SDK is
       always implicitly included). Both must be strings that reference a plug
       and a slot with the same interface in different SDKs, using the
       :samp:`<SDK>:<PLUG>` format.


Any entry in :samp:`sdks` must be named after an existing SDK
that is available from the SDK store.
Each SDK is described with the following keys:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 1 1 7

   * - Key
     - Value
     - Description

   * - :samp:`channel` (required)
     - string
     - SDK version to retrieve during
       :ref:`launch <ref_workshop_launch>`
       and
       :ref:`refresh <ref_workshop_refresh>`
       operations.

       It uses a
       `snap-like format <https://snapcraft.io/docs/channels>`__
       of :samp:`<TRACK>/<RISK>`
       without the :samp:`<BRANCH>` part.

   * - :samp:`plugs`
     - object
     - Lists plug bindings or additional plug definitions under the SDK.

       - A plug binding must name an existing plug in the SDK
         and set a single :samp:`bind` attribute
         that references a plug of the same interface in a different SDK
         using the :samp:`<SDK>:<PLUG>` format.

       - A plug definition must specify the :samp:`interface`
         and the relevant attributes.
         The only interface with additional attributes is :samp:`mount`;
         it requires the :samp:`workshop-target` property
         to specify a path inside the workshop
         to be used as the plug's target directory.

   * - :samp:`slots`
     - object
     - Defines additional slots under the SDK;
       each entry must specify the :samp:`interface`
       and the relevant attributes.

       The only interface with additional attributes is :samp:`mount`;
       it requires the :samp:`workshop-source` property
       to specify a path inside the workshop
       for the slot's source directory;
       :file:`/project` or :envvar:`$SDK`-based paths can be used;
       :envvar:`$SDK` expands into the SDK's installation path in the workshop.


JSON Schema
-----------

The following
`JSON Schema`
formalises the description above:

.. dropdown:: Workshop definition schema

   .. literalinclude:: schema.json
      :language: json


Examples
--------

This YAML file defines a :samp:`golang` workshop
with a single :samp:`go` SDK
from the :samp:`latest/stable` channel:

.. code-block:: yaml
   :caption: .workshop/workshop.golang.yaml

   name: golang
   base: ubuntu@22.04
   sdks:
     go:
       channel: latest/stable


This YAML file defines a :samp:`go-dev` workshop
that uses two SDKs, :samp:`go` and :samp:`dev-tunnel`;
the :samp:`data` plug defined by the :samp:`dev-tunnel` SDK
is bound to the :samp:`mod-cache` plug of the :samp:`go` SDK:

.. code-block:: yaml
   :caption: .workshop/workshop.go-dev.yaml

   name: go-dev
   base: ubuntu@22.04
   sdks:
     go:
       channel: latest/candidate
     dev-tunnel:
       channel: latest/edge
       plugs:
         data:
           bind: go:mod-cache


This YAML file, besides using the :samp:`tensorflow` and :samp:`cuda` SDKs,
defines an additional slot under the system SDK, a plug under :samp:`tensorflow`
and two connections:

- One that connects the :samp:`tensorflow:images` plug
  to the newly defined :samp:`system:images` slot.

- Another that connects the :samp:`tensorflow:cuda` plug
  to the pre-existing :samp:`cuda:libs`.

.. code-block:: yaml
   :caption: .workshop/workshop.digits-cuda.yaml

   base: ubuntu@22.04
   name: digits-cuda
   sdks:
     system:
       slots:
         images:
           interface: mount
           workshop-source: /project/training-data/low-res
     tensorflow:
       channel: latest/stable
       plugs:
         cuda:
           interface: mount
           workshop-target: /usr/local/cuda/lib64
     cuda:
       channel: latest/stable
   connections:
     - plug: tensorflow:cuda
       slot: cuda:libs
     - plug: tensorflow:images
       slot: system:images


See also
--------

Explanation:

- :ref:`exp_sdk`
- :ref:`exp_base`
- :ref:`exp_system_sdk`
- :ref:`exp_workshop_def`


Reference:

- :ref:`ref_workshop_info`
