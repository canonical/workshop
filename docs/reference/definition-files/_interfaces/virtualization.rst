..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Virtualization interface
~~~~~~~~~~~~~~~~~~~~~~~~~~

The virtualization interface exposes the host character devices required to run
hardware-accelerated virtual machines (KVM) inside a workshop.

- Plug attributes: none.
- Plug name: must be :samp:`virtualization`.
- Plug owner: any regular SDK; not the system SDK.
- Slot: the system SDK provides a single :samp:`system:virtualization` slot.
  Other SDKs cannot declare virtualization slots.

When connected, the following host devices are passed through to the workshop as
:samp:`unix-char` devices, owned by the :samp:`kvm` group with mode
:samp:`0660`, and the workshop user is made a member of that group:

- :file:`/dev/kvm`
- :file:`/dev/vhost-net`
- :file:`/dev/vhost-vsock`
- :file:`/dev/vsock`

Devices that are not present on the host (for example when the :samp:`vsock`
kernel modules are not loaded) are skipped and do not prevent the workshop from
starting.
