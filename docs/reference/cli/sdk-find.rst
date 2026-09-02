.. _ref_sdk_find:


.. meta::
   :description: Reference documentation for the 'sdk find' command

sdk find
--------

.. @artefact sdk find

Search the Store for SDKs.

.. rubric:: Usage

.. code-block:: console

   $ sdk find <QUERY> [flags]

.. rubric:: Description

Search the Store for SDKs matching the given query.
The query can match the SDK's name, title, summary, description, or publisher.

Notes:

- Only the latest release of the SDK is shown.
- To view more details for one of the SDKs, use "sdk info".
- To list SDKs on the local system, use "sdk list".

.. rubric:: Examples

Search for SDKs matching a single keyword:

.. code-block:: console

   $ sdk find openvino

Combine multiple words into a single query:

.. code-block:: console

   $ sdk find jupyter notebooks

Hide the table header in the output:

.. code-block:: console

   $ sdk find openvino --no-headers


.. rubric:: Flags

--no-headers

   Hide table headers.

