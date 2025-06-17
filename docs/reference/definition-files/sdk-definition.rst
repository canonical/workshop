.. _ref_sdk_definition:

SDK definition
==============

.. @artefact SDK
.. @artefact SDK definition

Filename convention
-------------------

.. @artefact sdkcraft (CLI)

The name of the SDK definition file must be :file:`sdk.yaml`;
the file is usually created using the :command:`sdkcraft init` command
in the source directory when :ref:`building an SDK <how_sdkcraft>`.


Structure
---------

The definition in the file must be written in
`YAML <https://yaml.org/>`__
and include these top-level fields:
:samp:`name`, :samp:`base`, :samp:`version`, :samp:`summary`,
:samp:`description`, :samp:`license`, :samp:`platforms` and :samp:`parts`.
The :samp:`plugs` field is optional.

.. @artefact SDK base image

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 1 1 7

   * - Key
     - Value
     - Description

   * - :samp:`name`
     - string
     - SDK's name, used to reference it in the workshop definition.


   * - :samp:`base`
     - string
     - SDK's base image
       that provides the underlying OS capabilities.

       It can be :samp:`ubuntu@20.04`, :samp:`ubuntu@22.04`
       or :samp:`ubuntu@24.04`.

   * - :samp:`version`
     - string
     - SDK's arbitrary version;
       semantic versioning is recommended.

       .. note::

          Use quotes to avoid potential data type mismatches:
          without them, :samp:`'1.0'` can be interpreted as a number,
          for example.


   * - :samp:`summary`
     - string
     - A short one-line summary of up to 79 characters.


   * - :samp:`description`
     - string
     - A longer, more detailed description of the SDK, up to one hundred words.


   * - :samp:`license`
     - string
     - Name of the software license under which the SDK is distributed.

       .. note::

          Make sure it matches the individual components of the SDK.


   * - :samp:`platforms`
     - object
     - Lists individual architectures that the SDK supports.


   * - :samp:`parts`
     - object
     - See :ref:`ref_sdk_parts` for a detailed discussion.


   * - :samp:`plugs`
     - object
     - See :ref:`ref_sdk_plugs_slots` for a detailed discussion.

   * - :samp:`slots`
     - object
     - See :ref:`ref_sdk_plugs_slots` for a detailed discussion.


For example:

.. code-block:: yaml
   :caption: sdk.yaml

    name: go
    title: Go SDK
    base: ubuntu@22.04
    summary: The Go programming language
    description: |
      Go is an open source programming language that enables the production
      of simple, efficient and reliable software at scale.
    version: '0.1'
    license: LGPL-2.1
    platforms:
        amd64:

    parts:
      go-part:
        plugin: nil

    plugs:
      mod-cache:
        interface: mount
        workshop-target: /home/workshop/go/pkg/mod

   slots:
      tools:
        interface:  mount
        workshop-source: $SDK/go


JSON Schema
-----------

The following
`JSON Schema`
formalizes the description above:

.. @artefact SDK schema

.. dropdown:: SDK definition schema

   .. literalinclude:: schema-sdk.json
      :language: json

Examples
--------

This YAML file defines a simple :samp:`go` SDK:

.. code-block:: yaml
   :caption: sdk.yaml

   name: go
   base: ubuntu@24.04
   version: '0.1'
   summary: Go SDK
   description: |
     This is my Go SDK description.
   license: GPL-3.0
   platforms:
     amd64:

   parts:
     my-part:
       plugin: nil


This is a more elaborate example of an SDK
that uses several :ref:`plugs <ref_sdk_plugs_slots>` and multiple platforms:

.. code-block:: yaml
   :caption: sdk.yaml

   name: ros2
   title: The ROS 2 SDK
   base: ubuntu@24.04
   version: "0.1"
   summary: The strictly necessary ROS 2 development environment for your project.
   description: |
     The ROS 2 SDK creates a minimum viable development environment
     for your ROS 2 project.
     It sets up a bare-bones ROS 2 workspace
     before installing all of the dependencies
     for the ROS 2 project mounted by workshop.
   
     A developer can thus connect to the workshop
     to immediately build the project.
   license: LGPL-2.1
   platforms:
     amd64:
     arm64:
   
   parts:
     ros2-part:
       plugin: nil
   
   plugs:
     ros-cache:
       interface: mount
       workshop-target: /home/workshop/.ros
   
     colcon-artifacts:
       interface: mount
       workshop-target: /home/workshop/colcon
   
     gpu:
       interface: gpu


See also
--------

How-to guides:

- :ref:`how_sdkcraft`


Reference:

- :ref:`ref_sdk_internals`
- :ref:`ref_workshop_definition`
