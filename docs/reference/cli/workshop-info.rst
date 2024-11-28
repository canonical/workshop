.. _ref_workshop_info:

workshop info
-------------

Print the current status and details of a workshop as YAML.

.. rubric:: Synopsis

.. code-block:: console

   workshop info <WORKSHOP> [flags]

.. rubric:: Description


This command outputs the basic settings, current status and individual SDK
details for a workshop, formatting them as YAML. Specifically, it prints:

- Essential workshop attributes, such as name, base and project directory

- Current status (e.g. *Ready*, *Pending*, *Off*) and notes for the workshop

- Individual SDK details, such as name, channel, installation date and revision

- Currently mounted content interface plugs


Notes:

- Avoid assumptions based on SDK channels: 'latest/stable' may be neither


.. rubric:: Examples

.. code-block:: console
   
   # List details for the 'nimble' workshop in the current project directory
   workshop info nimble
