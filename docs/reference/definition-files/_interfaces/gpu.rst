..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

GPU interface
~~~~~~~~~~~~~

.. @artefact GPU interface

The GPU interface exposes host GPU devices.

- Plug attributes: none.
- Plug name: must be :samp:`gpu`.
- Plug owner: any regular SDK; not the system SDK.
- Slot: the system SDK provides a single :samp:`system:gpu` slot.
  Other SDKs cannot declare GPU slots.
