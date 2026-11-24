..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Custom device interface
~~~~~~~~~~~~~~~~~~~~~~~

The custom device interface exposes host devices
that belong to a Linux kernel subsystem.

A custom device plug is described by these attributes:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`subsystem` (optional)
     - string
     - The Linux kernel subsystem of the host devices to expose,
       for example :samp:`input`, :samp:`tty`, or :samp:`usb`.

   * - :samp:`vendorid` (optional)
     - string
     - Restrict the exposed devices to those with this vendor ID,
       quoted so it is read as a string, for example :samp:`"0403"`.

   * - :samp:`productid` (optional)
     - string
     - Restrict the exposed devices to those with this product ID,
       quoted so it is read as a string, for example :samp:`"6001"`.

At least one of :samp:`subsystem`, :samp:`vendorid`, or :samp:`productid`
must be set.
Setting :samp:`productid` also requires :samp:`vendorid`,
since a product ID is only meaningful within a vendor's namespace.

Plug owner: any regular SDK; not the system SDK.

Slot: the system SDK provides a single :samp:`system:custom-device` slot.
Other SDKs cannot declare custom device slots.
