.. _ref_workshop_actions:


.. meta::
   :description: Reference documentation for the 'workshop actions' command

workshop actions
----------------

.. @artefact workshop actions

List the named actions defined in a workshop.

.. rubric:: Usage

.. code-block:: console

   $ workshop actions [<WORKSHOP>] [flags]

.. rubric:: Description

This command enumerates all actions in the workshop, printing a YAML map.

.. rubric:: Examples

List actions for the "nimble" workshop in the current project directory:

.. code-block:: console

   $ workshop actions nimble

The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop actions

