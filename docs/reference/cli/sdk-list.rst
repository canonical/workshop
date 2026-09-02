.. _ref_sdk_list:


.. meta::
   :description: Reference documentation for the 'sdk list' command

sdk list
--------

.. @artefact sdk list

List SDK volumes available on this machine.

.. rubric:: Usage

.. code-block:: console

   $ sdk list [flags]

.. rubric:: Description

This command lists all local SDK volumes.

Use it to enumerate the SDK revisions currently stored on the system.
Only volumes are reported, not the workshops that use them.

Notes:

- For per-workshop information, use "workshop info".
- Multiple entries may appear for a single SDK
  if several revisions are present simultaneously.

.. rubric:: Examples

List all local SDK volumes:

.. code-block:: console

   $ sdk list

Hide the table header in the output:

.. code-block:: console

   $ sdk list --no-headers


.. rubric:: Flags

--no-headers

   Hide table headers.

