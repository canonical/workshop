.. meta::
   :description: Overview of the YAML definition files used by Workshop and
                 SDKcraft, with links to the reference page for each file.

SDK and workshop definition files
=================================

.. @artefact SDK
.. @artefact workshop definition
.. @artefact SDK definition

Three YAML files describe what a workshop is
and how the SDKs inside it behave.
Each file has one authoritative reference page:

- :file:`workshop.yaml` is authored by workshop users;
  |ws_markup| reads it to launch a workshop.
  See :ref:`ref_workshop_definition`.
- :file:`sdk.yaml` ships inside SDK packages;
  |ws_markup| reads it at install time.
  Also authored directly for in-project SDKs.
  See :ref:`ref_sdk_definition`.
- :file:`sdkcraft.yaml` is authored by SDK publishers;
  |sdk_markup| reads it to build an SDK package,
  which embeds the generated :file:`sdk.yaml`.
  See :ref:`ref_sdkcraft_definition`.

.. toctree::
   :maxdepth: 1

   workshop-definition
   sdk-definition
   sdkcraft-definition


See also
--------

Explanation:

- :ref:`exp_in_project_sdk`
- :ref:`exp_sdk_definition`
- :ref:`exp_workshop_definition`
