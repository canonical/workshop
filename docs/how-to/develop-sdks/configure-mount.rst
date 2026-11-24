.. _how_configure_mount:

.. meta::
   :description: Configure ownership and permissions on mount interface plugs
                 by setting uid, gid, mode, and read-only attributes,
                 overriding the path-based defaults that Workshop applies.

How to configure a mount
========================

.. @tests in tests/docs-how-to/configure-mount/task.yaml

This guide shows how to tune ownership and permissions
on a :samp:`mount` interface plug,
overriding the defaults that |ws_markup| derives from the target path.

By default, |ws_markup| applies the following rules
when a mount plug omits ownership and permission attributes.
The attributes take effect when |ws_markup| creates the target path
and any missing parent directories;
a path that already exists keeps its ownership and permissions.

- :samp:`uid` and :samp:`gid` default to :samp:`1000`
  when :samp:`workshop-target` is at or below
  :file:`/home/workshop`,
  :file:`/project`,
  or :file:`/run/user/1000`.
  Otherwise, both default to :samp:`0` (root).

- :samp:`mode` defaults to :samp:`0o755` when the :samp:`uid`,
  set explicitly or resolved by the previous rule, is :samp:`0` (root),
  and to :samp:`0o775` otherwise.

- :samp:`read-only` defaults to :samp:`false`.


Override the defaults when a mount lives outside the user-owned paths,
when a mount stores secrets that should not be world-readable,
or when the SDK should not be able to write to the mount.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- An :file:`sdkcraft.yaml` that declares at least one mount plug.
  :ref:`ref_sdk_plugs_slots` covers the declarations.

The attributes below extend the plug entries
under the top-level :samp:`plugs:` key.


Set explicit ownership outside the user-owned paths
----------------------------------------------------

For a mount whose target lives outside :file:`/home/workshop`,
:file:`/project`, or :file:`/run/user/1000`,
the ownership defaults to root.
If the :samp:`workshop` user needs to read or write the mount,
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


An explicit :samp:`uid` doesn't change the default for :samp:`gid`,
which still follows the target path,
so set both attributes.
The default :samp:`mode`, in contrast, follows the resolved owner
and becomes :samp:`0o775` here.


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


The owner keeps the path-based default
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

After adding the SDK to a workshop definition and launching the workshop,
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

- :ref:`how_resolve_plug_conflicts`


Reference:

- :ref:`ref_mount_interface`
- :ref:`ref_sdk_plugs_slots`


Tutorial:

- :ref:`tut_craft_sdks`
