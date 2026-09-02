.. _ref_sdk_info:


.. meta::
   :description: Reference documentation for the 'sdk info' command

sdk info
--------

.. @artefact sdk info

Show SDK info.

.. rubric:: Usage

.. code-block:: console

   $ sdk info <SDK> [flags]

.. rubric:: Description

Prints the SDK's metadata,
shows the revisions currently available in the SDK Store,
and lists workshops where the SDK is installed.

Notes:

- The output shows the SDK's build date.
- For an overview of SDK volumes, use "sdk list".
- For per-workshop information, use "workshop info".

.. rubric:: Examples

Show metadata, Store channels, and local installations for the "openvino" SDK:

.. code-block:: console

   $ sdk info openvino

Restrict the Store channels to a specific base:

.. code-block:: console

   $ sdk info openvino --base ubuntu@24.04

Show the channels for every supported architecture:

.. code-block:: console

   $ sdk info openvino --arch all


.. rubric:: Flags

--arch

   Show SDKs compatible with a different architecture (or "all").

--base

   Show SDKs compatible with a specific base.

