..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Desktop interface
~~~~~~~~~~~~~~~~~

.. @artefact desktop interface

The desktop interface exposes the host display server.

- Plug attributes: none.
- Plug name: must be :samp:`desktop`.
- Plug owner: any regular SDK; not the system SDK.
- Slot: the system SDK provides a single :samp:`system:desktop` slot. Other SDKs cannot declare desktop slots.
