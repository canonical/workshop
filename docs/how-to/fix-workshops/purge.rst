.. _how_purge:

.. meta::
   :description: How-to guide on purging malfunctioning workshops, covering how to
                 copy files out of them first, then remove containers,
                 metadata, and files thoroughly.

How to purge malfunctioning workshops
=====================================

.. @tests not applicable: full system purge not safe in CI

Workshops can sometimes become unresponsive,
encounter errors during start or stop operations,
or become orphaned if their project directory is removed prematurely.

A thorough purge involves removing the workshop's containers, metadata,
and files in a deliberate sequence.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- Identified the workshop and project you intend to purge.

- Copied out any files you need from inside the workshop,
  as described in the next section;
  manual cleanup can permanently delete containers, metadata, and files.

- Checked that you can run :command:`sudo` and :command:`lxc`
  for manual LXD cleanup.


.. _how_purge_before_removing:

Before removing a workshop
--------------------------

Removing a workshop deletes everything inside it,
along with the host directories
that |ws_markup| allocated for its mount plugs under
:file:`~/.local/share/workshop/id/<PROJECT-ID>/<WORKSHOP>/mount/`;
copy any files you need from there to another place on the host first.
Only host directories that you manage yourself survive,
such as the project directory at :file:`/project`
or a directory relocated with :command:`workshop remount`.
Files elsewhere in the workshop,
such as the :samp:`workshop` user's home directory,
are lost.

If the workshop still runs,
open a shell in it and copy the files you need
into a new directory in the project:

.. code-block:: console

   $ workshop shell <WORKSHOP>

   workshop@<WORKSHOP>:/project$ mkdir /project/workshop-recovery
   workshop@<WORKSHOP>:/project$ cp -a ~/<PATH> /project/workshop-recovery/


If the workshop doesn't start or you can't open a shell in it,
copy the files out of its LXD instance instead,
as described in the next subsection.


.. _how_purge_copy_out:

Copy files out of a broken workshop
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Each workshop runs as an LXD instance named :samp:`<WORKSHOP>-<PROJECT-ID>`
in the :samp:`workshop.<USERNAME>` LXD project,
where :samp:`<USERNAME>` is your system username or,
if the username can't be used in an LXD project name,
your numeric user ID (:command:`id -u`).
Find the instance and check whether it's a container or a virtual machine:

.. code-block:: console

   $ lxc list --project workshop.<USERNAME> --columns nst


For a container,
pull the files into a new directory on the host:

.. code-block:: console

   $ mkdir ~/workshop-recovery
   $ lxc file pull --recursive --project workshop.<USERNAME> <INSTANCE>/home/workshop/<PATH> ~/workshop-recovery/


Run :command:`lxc` without :command:`sudo` here,
so that the copies belong to your host user;
this requires membership in the :samp:`lxd` group.
Symbolic links are copied as links;
:command:`lxc` doesn't follow them out of the container.

For a virtual machine,
:command:`lxc file pull` works only while the VM runs
and its LXD agent responds.
Otherwise, mount the disk of the broken VM in another VM workshop
and copy the files from there.
Don't mount it on the host:
anything inside the VM could have written to that disk,
and the host kernel's filesystem code isn't built to withstand
a disk that was altered on purpose.
Mounting such a disk can crash the host or give an attacker control of it.
A VM keeps that damage away from the host kernel,
but the VM can still change the host directories it mounts read-write,
including its project directory,
so recover into a VM whose project directory is empty.

#. In a new, empty directory,
   create a VM workshop to recover the files with and launch it,
   as described in :ref:`how_use_virtual_machines`:

   .. code-block:: console

      $ mkdir ~/vm-recovery
      $ workshop init <RECOVERY-WORKSHOP> --runtime lxd-vm --project ~/vm-recovery
      $ workshop launch <RECOVERY-WORKSHOP> --project ~/vm-recovery


   Don't add SDKs to its definition;
   without them, no interfaces connect the VM to the host.
   Find its instance name the same way as for the broken workshop.

#. Stop the broken VM if it's still running:

   .. code-block:: console

      $ lxc stop --force --project workshop.<USERNAME> <INSTANCE>


#. Allow its root volume to be attached to another instance,
   then attach it to the recovery VM:

   .. code-block:: console

      $ lxc storage volume set --project workshop.<USERNAME> workshop virtual-machine/<INSTANCE> security.shared=true
      $ lxc config device add --project workshop.<USERNAME> <RECOVERY-INSTANCE> recovery disk pool=workshop source=<INSTANCE> source.type=virtual-machine


#. Open a shell in the recovery workshop
   and find the attached disk;
   it's the one whose partitions have no mount points:

   .. code-block:: console

      $ workshop shell <RECOVERY-WORKSHOP> --project ~/vm-recovery

      workshop@<RECOVERY-WORKSHOP>:/project$ lsblk -f

        NAME    FSTYPE FSVER LABEL           UUID                                 FSAVAIL FSUSE% MOUNTPOINTS
        sda
        ├─sda1  ext4   1.0   cloudimg-rootfs 59134bd0-a76f-4ea0-8e4a-770b2e1eafc2      7G    19% /
        ├─sda14
        ├─sda15 vfat   FAT32 UEFI            0E9D-3FD8                              98.2M     6% /boot/efi
        └─sda16 ext4   1.0   BOOT            b4f0deba-ce7d-45dc-9999-0427f2720320  755.4M     7% /boot
        sdb
        ├─sdb1  ext4   1.0   cloudimg-rootfs 59134bd0-a76f-4ea0-8e4a-770b2e1eafc2
        ├─sdb14
        ├─sdb15 vfat   FAT32 UEFI            0E9D-3FD8
        └─sdb16 ext4   1.0   BOOT            b4f0deba-ce7d-45dc-9999-0427f2720320


#. Mount the :samp:`cloudimg-rootfs` partition of that disk read-only
   and copy the files you need into a new directory in the project:

   .. code-block:: console

      workshop@<RECOVERY-WORKSHOP>:/project$ sudo mkdir /mnt/recovery
      workshop@<RECOVERY-WORKSHOP>:/project$ sudo mount -o ro /dev/sdb1 /mnt/recovery
      workshop@<RECOVERY-WORKSHOP>:/project$ mkdir /project/workshop-recovery
      workshop@<RECOVERY-WORKSHOP>:/project$ cp -a /mnt/recovery/home/workshop/<PATH> /project/workshop-recovery/


#. Unmount the disk and detach it from the recovery VM:

   .. code-block:: console

      workshop@<RECOVERY-WORKSHOP>:/project$ sudo umount /mnt/recovery
      workshop@<RECOVERY-WORKSHOP>:/project$ exit

      $ lxc config device remove --project workshop.<USERNAME> <RECOVERY-INSTANCE> recovery


   The recovered files are in :file:`~/vm-recovery/workshop-recovery/` on the host.


Standard removal procedure
--------------------------

The primary command for removing a workshop is:

.. code-block:: console

   $ workshop remove <WORKSHOP>


This command is designed to:

- Stop the workshop if it is running.
- Delete the underlying LXD container.
- Remove associated workshop data and cache directories.
- Clean up related LXD profiles and remove device mounts.


Always attempt this command first.
If it completes successfully, your workshop should be purged.
You can verify the outcome by running :command:`workshop list`.


If standard procedure fails
---------------------------

You may need manual intervention if:

- :command:`workshop remove` fails with an error.

- The workshop is still listed
  by :command:`workshop list` or :command:`workshop list --global`
  after a remove attempt.

- The workshop's project directory had been deleted
  before the workshop was removed,
  leaving the workshop orphaned.

- The workshop is in an unrecoverable error state.

- The workshop's container is still running or in an error state,
  preventing the standard removal flow from completing.


For an orphaned workshop,
first try :ref:`removing it by its former project path <how_purge_orphaned>`,
which doesn't require touching LXD.
In other cases, or if that fails,
manually clean up the workshop's resources,
interacting directly with LXD and the workshop's snap data;
start by :ref:`finding the LXD project <how_purge_manual>`.


.. _how_purge_orphaned:

Remove an orphaned workshop
~~~~~~~~~~~~~~~~~~~~~~~~~~~

If a project directory is deleted before its workshops are removed,
the workshops become orphaned:
their containers and stored state remain on the host,
and the daemon reports them in the *Error* state
with a :samp:`missing-project` note:

.. code-block:: console

   $ workshop list --global

     PROJECT            WORKSHOP  STATUS  NOTES
     ~/projects/nimble  nimble    Error   missing-project



The daemon keeps tracking the project's original location
until its last workshop is removed,
so you can remove the workshop
by passing that location to the :option:`!--project` option;
the directory doesn't need to exist:

#. Remove the workshop,
   pointing at the project's former path
   as shown in the :samp:`PROJECT` column:

   .. code-block:: console

      $ workshop remove --project ~/projects/nimble nimble


   The path must be the project's root directory;
   subdirectories of a deleted project aren't recognized.

#. Verify the removal:

   .. code-block:: console

      $ workshop list --global


.. note::

   Removal isn't the only option:
   if you recreate the directory at the same absolute path,
   running any workshop command against it
   re-associates it with the original project,
   restoring the hidden :file:`.workshop.lock` file.
   To recover the workshop instead,
   restore the project's content (e.g. from a repository)
   and continue using it.


.. _how_purge_manual:

Find LXD project
~~~~~~~~~~~~~~~~

Workshop creates LXD projects named :samp:`workshop.<USERNAME>`,
where :samp:`<USERNAME>` is your system username.
If the username can't be used in an LXD project name
(e.g. if it contains special characters such as :samp:`@`),
your numeric user ID is used instead (:command:`id -u`).
You'll also need your username for some paths.


Clean up LXD resources
~~~~~~~~~~~~~~~~~~~~~~

Refer to the :ref:`how_troubleshoot_lxc` section in the troubleshooting guide
for initial steps on listing and deleting orphaned LXD containers, e.g.:

.. code-block:: console

   $ sudo lxc list --all-projects | grep workshop.<USERNAME>
   $ sudo lxc delete --project workshop.<USERNAME> <CONTAINER> --force


To ensure there are no backup copies of the workshop remaining,
check the :samp:`workshop-snapshots.<USERNAME>` project as well:

.. code-block:: console

   $ sudo lxc list --all-projects | grep workshop-snapshots.<USERNAME>
   $ sudo lxc delete --project workshop-snapshots.<USERNAME> <CONTAINER> --force


In addition to containers,
you may need to clean up associated LXD profiles.


LXD profiles
````````````

Workshops create an LXD profile for each SDK they use.
These profiles are named :samp:`<CONTAINER>-<SDK>`.
If a workshop container wasn't cleanly removed,
its profiles might remain.

- List profiles for your workshop user project:

  .. code-block:: console

     $ sudo lxc profile list --project workshop.<USERNAME>


- Inspect a specific profile:

  .. code-block:: console

     $ sudo lxc profile show --project workshop.<USERNAME> <PROFILE>


- Delete an orphaned profile.
  To ensure it's not in use by other valid workshops,
  list all containers in the project firstly:

  .. code-block:: console

     $ sudo lxc list --project workshop.<USERNAME>


  Then, for each container that should remain,
  check its configuration to see which profiles it uses:

  .. code-block:: console

     $ sudo lxc config show --project workshop.<USERNAME> <CONTAINER>

  Look for the :samp:`profiles` key in the output.

  If the :samp:`<PROFILE>` you intend to delete
  is not listed for any relevant containers,
  it should be safe to remove:

  .. code-block:: console

     $ sudo lxc profile delete --project workshop.<USERNAME> <PROFILE>


- To delete an orphaned profile, check the :samp:`USED BY` column
  in the output of the :command:`lxc profile list` command.
  If the count is zero,
  the profile is not used by any containers and can be safely removed.


Remove leftover host directories
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Deleting containers with :command:`lxc` doesn't remove the state
that |ws_markup| stores for them on the host.
This state is keyed by project ID,
which is the final dash-separated segment of the container name;
for example, :samp:`ec275767` for a container named :samp:`nimble-ec275767`.

Check these locations for leftover directories,
removing them if present:

.. code-block:: console

   $ rm -rf ~/.local/share/workshop/id/<PROJECT-ID>
   $ sudo rm -rf /var/snap/workshop/current/id/<PROJECT-ID>
   $ sudo rm -rf /var/snap/workshop/common/workshop/cache/id/<PROJECT-ID>


Aggressive cleanup
------------------

If previous steps haven't resolved the issue,
or if :command:`workshop list` still shows remnants,
the most aggressive cleanup method is to completely purge the |ws_markup| snap.
This executes the snap's :samp:`remove` hook,
which is designed to clean up all associated data and resources.

To purge the snap and all its data, run the following command:

.. code-block:: console

   $ sudo snap remove workshop --purge


This will remove all workshop configurations, containers, LXD profiles,
and storage pools managed by |ws_markup|.

After the command completes, you can reinstall the snap.

.. warning::

   This is a highly destructive operation that removes all workshops
   for all users on the system. It should only be used as a last resort.
   You will need to reinstall |ws_markup| to use it again.


Final checks
------------

After performing manual cleanup steps:

- Run :command:`workshop list --global`
  to check if the malfunctioning workshop is no longer listed.

- Run :command:`sudo lxc list --all-projects`
  to ensure no unexpected LXD resources remain.


If issues persist,
consider seeking community support,
or reporting a bug with detailed logs and steps taken:
:ref:`project_community`.


See also
--------

Explanation:

- :ref:`exp_projects`


How-to guides:

- :ref:`how_add_mounts`
- :ref:`how_debug_issues_workshops`
- :ref:`how_troubleshoot`
- :ref:`how_use_virtual_machines`


Reference:

- :ref:`ref_workshop_list`
- :ref:`ref_workshop_remount`
- :ref:`ref_workshop_remove`
- :ref:`ref_workshop_shell`
