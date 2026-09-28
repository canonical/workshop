.. _ref_workshop_sketches:


.. meta::
   :description: Reference documentation for the 'workshop sketches' command

workshop sketches
-----------------

.. @artefact workshop sketches

List project sketch SDKs.

.. rubric:: Usage

.. code-block:: console

   $ workshop sketches [flags]

.. rubric:: Description

This command enumerates all sketches in the project, printing a compact list:

- Project:  absolute pathname of the project

- Workshop: workshop name, as set in its definition

- Rev:      sketch SDK revision, if present

- Notes:    current, stashed, or both

.. rubric:: Examples

List the sketches in the current project directory:

.. code-block:: console

   $ workshop sketches


.. rubric:: Flags

--no-headers

   Hide table headers.

