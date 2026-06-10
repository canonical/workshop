..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Custom device interface
~~~~~~~~~~~~~~~~~~~~~~~

.. @artefact custom-device interface

The custom device interface exposes host devices
that belong to a Linux kernel subsystem.

A custom device plug is described by this attribute:

.. @artefact custom-device interface attributes

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`subsystem` (required)
     - string
     - The Linux kernel subsystem of the host devices to expose,
       for example :samp:`input`, :samp:`tty`, or :samp:`usb`.

Plug owner: any regular SDK; not the system SDK.

Slot: the system SDK provides a single :samp:`system:custom-device` slot.
Other SDKs cannot declare custom device slots.
