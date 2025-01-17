.. _ref_workshop_sketches:

workshop sketches
-----------------

List sketches.

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


