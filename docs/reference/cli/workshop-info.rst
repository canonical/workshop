.. _ref_workshop_info:

workshop info
-------------

Print the current status and details of a workshop as YAML.

.. rubric:: Usage

.. code-block:: console

   $ workshop info [<WORKSHOP>] [flags]

.. rubric:: Description


This command outputs the basic settings, current status and individual SDK
details for a workshop, formatting them as YAML. Specifically, it prints:

- Essential workshop attributes, such as name, base and project directory

- Current status (e.g. 'Ready', 'Pending', 'Off') and notes for the workshop

- Individual SDK details, such as name, channel, installation date and revision

- Currently connected mount interface plugs


Notes:

- Avoid assumptions based on SDK channels: 'latest/stable' may be neither.


.. rubric:: Examples


List details for the 'nimble' workshop in the current project directory:

.. code-block:: console

   $ workshop info nimble


The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop info


