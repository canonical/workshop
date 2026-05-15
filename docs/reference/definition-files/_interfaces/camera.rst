..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Camera interface
~~~~~~~~~~~~~~~~

.. @artefact camera interface

The camera interface exposes a host camera device.

- Plug attributes: none.
- Plug name: must be :samp:`camera`.
- Plug owner: any regular SDK; not the system SDK.
- Slot: the system SDK provides a single :samp:`system:camera` slot. Other SDKs cannot declare camera slots.
