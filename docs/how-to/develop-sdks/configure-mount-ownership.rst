.. _how_configure_mount_ownership:

.. meta::
   :description: Configure ownership and permissions on mount interface plugs
                 by setting uid, gid, mode, and read-only attributes,
                 overriding the path-aware defaults that |ws_markup| applies.

How to configure mount ownership
================================

.. @artefact mount interface
.. @artefact interface plug

This guide shows how an SDK author tunes ownership and permissions
on a :samp:`mount` interface plug,
overriding the defaults that |ws_markup| derives from the target path.

By default, |ws_markup| applies the following rules
when a mount plug omits ownership and permission attributes:

- :samp:`uid` and :samp:`gid` default to :samp:`1000`
  when :samp:`workshop-target` is at or below
  :file:`/home/workshop`,
  :file:`/project`,
  or :file:`/run/user/1000`.
  Otherwise, both default to :samp:`0` (root).

- :samp:`mode` defaults to :samp:`0o775`
  when the owner is the :samp:`workshop` user,
  or :samp:`0o755` when the owner is root.
  The maximum allowed value is :samp:`0o777`.

- :samp:`read-only` defaults to :samp:`false`.


Override the defaults when a mount lives outside the path-aware home tree,
when a mount stores secrets that should not be world-readable,
or when the SDK should not be able to write to the mount.


Set explicit ownership outside the home tree
--------------------------------------------

For a mount whose target lives outside :file:`/home/workshop/`,
:file:`/project/`, or :file:`/run/user/1000/`,
the ownership defaults to root.
If the :samp:`workshop` user has to read or write the mount,
set :samp:`uid` and :samp:`gid` to :samp:`1000`:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 5-6

   plugs:
     state:
       interface: mount
       workshop-target: /var/lib/example
       uid: 1000
       gid: 1000


Tighten permissions for a private mount
---------------------------------------

For a mount that stores credentials, tokens, or other secrets,
tighten :samp:`mode` to :samp:`0o700`
so that only the owning user can access it:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 5

   plugs:
     secrets:
       interface: mount
       workshop-target: /home/workshop/.private-secrets
       mode: 0o700


The owner inherits the path-aware default
(here, the workshop user, since the target is under :file:`/home/workshop`).


Mark a mount read-only
----------------------

For a mount that should expose data without allowing the consumer to modify it,
set :samp:`read-only` to :samp:`true`:

.. code-block:: yaml
   :caption: sdkcraft.yaml
   :emphasize-lines: 5

   plugs:
     toolchain:
       interface: mount
       workshop-target: /home/workshop/.local/share/example-toolchain
       read-only: true


This is appropriate for shared resources
that the SDK consumes but never writes to.


Verify inside a workshop
------------------------

After packing the SDK, listing it in a workshop, and launching the workshop,
inspect the mounts to confirm the applied ownership and permissions.
The numeric uid and gid show how |ws_markup| resolved the attributes:

.. code-block:: console

   $ workshop exec dev -- ls -ldn /home/workshop/.private-secrets

     drwx------ 2 1000 1000 4096 May 14 10:32 /home/workshop/.private-secrets


For a read-only mount,
attempting to write returns an :samp:`EROFS` error:

.. code-block:: console

   $ workshop exec dev -- touch /home/workshop/.local/share/example-toolchain/probe

     touch: cannot touch '/home/workshop/.local/share/example-toolchain/probe': Read-only file system


See also
--------

Explanation:

- :ref:`exp_mount_interface`
- :ref:`exp_plugs_slots`


How-to guides:

- :ref:`how_declare_plugs_slots`
- :ref:`how_design_interface_layout`
- :ref:`how_bind_plug_slot_pairs_explicitly`


Reference:

- :ref:`ref_mount_interface`
- :ref:`ref_sdk_plugs_slots`
