..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Mount interface
~~~~~~~~~~~~~~~

.. @artefact mount interface
.. @artefact $SDK

The mount interface exposes a directory between a slot owner and a plug owner.

A mount plug is described by these attributes:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`workshop-target` (required)
     - string
     - Path inside the workshop used as the plug's target directory.
       Must be an absolute path;
       :envvar:`$SDK` expands to the SDK's installation path in the workshop.

   * - :samp:`mode`
     - integer
     - File permissions, in octal, applied when creating :samp:`workshop-target`
       and any missing parent directories.
       Defaults to :samp:`0o775` for regular users.
       When :samp:`uid` is zero, defaults to :samp:`0o755`.

   * - :samp:`uid`
     - integer
     - User ID applied when creating :samp:`workshop-target`
       and any missing parent directories.
       Defaults to :samp:`1000` when :samp:`workshop-target` is under
       :file:`/home/workshop/`, :file:`/project/`, or :file:`/run/user/1000/`.
       Defaults to :samp:`0` otherwise.

   * - :samp:`gid`
     - integer
     - Group ID applied when creating :samp:`workshop-target`
       and any missing parent directories.
       Defaults to :samp:`1000` or :samp:`0`
       by the same path rule as :samp:`uid`,
       even when :samp:`uid` is set explicitly.

   * - :samp:`read-only`
     - Boolean
     - Whether the target directory should be read-only.
       Defaults to :samp:`false`.

Plug owner: any regular SDK; not the system SDK.

The system SDK provides one mount slot, :samp:`system:mount`,
with a dynamic :samp:`host-source` attribute
that can be configured only at :ref:`remount <ref_workshop_remount>`.
It is the only mount slot whose source is on the host filesystem.

A mount slot on a regular SDK is described by this attribute:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`workshop-source` (required)
     - string
     - Path inside the workshop used as the slot's source directory.
       Must be an absolute path;
       :envvar:`$SDK` expands to the SDK's installation path in the workshop.
