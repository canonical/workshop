.. _ref_workshop_init:


.. meta::
   :description: Reference documentation for the 'workshop init' command

workshop init
-------------

.. @artefact workshop init

Create a new workshop definition in the project directory.

.. rubric:: Usage

.. code-block:: console

   $ workshop init <NAME> --sdks <SDKs> [--base <BASE>] [flags]

.. rubric:: Description


Create a new workshop definition file in the project's .workshop/ directory.

The NAME argument sets the workshop name. The command creates a named
workshop file at .workshop/<NAME>.yaml. This fails if a workshop with
the same name already exists.

SDKs are specified as a comma-separated list. Each SDK entry can optionally
include a channel using the <name>/<channel> syntax (e.g., "go/1.26/stable").


.. rubric:: Examples


Create a workshop called "dev" with the Go and UV SDKs:

.. code-block:: console

   $ workshop init dev --sdks go,uv


Create a workshop with a specific SDK channel:

.. code-block:: console

   $ workshop init dev --sdks go/1.26/stable


Create a workshop using a specific base:

.. code-block:: console

   $ workshop init dev --sdks go --base ubuntu@22.04



.. rubric:: Flags


--base

   Base image for the workshop.


--sdks

   Comma-separated list of SDKs (e.g., "go,uv/latest/stable").




