.. _ref_workshop_info:

workshop info
-------------

Print the current status and details of a workshop as YAML

Synopsis
~~~~~~~~


This command outputs the basic settings, current status and individual SDK
details for a workshop, formatting them as YAML. Specifically, it prints:

- Essential workshop attributes, such as name, base and project directory

- Current status (e.g. *Ready*, *Pending*, *Off*) and notes for the workshop

- Individual SDK details, such as name, channel, installation date and revision

- Currently mounted content interface plugs


Notes:

- Avoid assumptions based on SDK channels: 'latest/stable' may be neither


.. code-block:: console

   workshop info <WORKSHOP> [flags]

