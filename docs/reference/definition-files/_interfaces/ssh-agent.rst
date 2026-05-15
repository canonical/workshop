..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

SSH interface
~~~~~~~~~~~~~

.. @artefact SSH interface

The SSH interface exposes the user's SSH agent socket.

- Plug attributes: none.
- Plug name: must be :samp:`ssh-agent`.
- Plug owner: any regular SDK; not the system SDK.
- Slot: the system SDK provides a single :samp:`system:ssh-agent` slot. Other SDKs cannot declare SSH slots.
