.. _how_purge:

How to purge malfunctioning workshops
=====================================

Workshops can sometimes become unresponsive,
encounter errors during start or stop operations,
or become orphaned if their project directory is removed prematurely.

This guide provides comprehensive steps
to thoroughly purge such workshops,
ensuring the removal of their containers, metadata, and files.


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
  before you attempted to remove the workshop,
  potentially orphaning LXD resources.

- The workshop is in an unrecoverable error state.

- The workshop's container is still running or in an error state,
  preventing standard.


If the standard procedure is ineffective for any of the above reasons,
you will need to manually clean up the workshop's resources.
For this, you interact directly with LXD and the workshop's snap data.


Find LXD project
~~~~~~~~~~~~~~~~

Workshop creates LXD projects named :samp:`workshop.<USERNAME>`,
where :samp:`<USERNAME>` is your system username.
You'll also need your username for some paths.


Clean up LXD resources
~~~~~~~~~~~~~~~~~~~~~~

Refer to the :ref:`how_troubleshoot_lxc` section in the troubleshooting guide
for initial steps on listing and deleting orphaned LXD containers, e.g.:

.. code-block:: console

   $ sudo lxc list --all-projects | grep workshop.<USERNAME>
   $ sudo lxc delete --project workshop.<USERNAME> <CONTAINER> --force


To ensure there are no backup copies of the workshop remaining,
check the :samp:`workshop-stash.<USERNAME>` project as well:

.. code-block:: console

   $ sudo lxc list --all-projects | grep workshop-stash.<USERNAME>
   $ sudo lxc delete --project workshop-stash.<USERNAME> <CONTAINER> --force


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
  in the output of the :command:`lxc profile list` command.
  If the count is zero,
  the profile is not used by any containers and can be safely removed.


Aggressive cleanup
------------------

If previous steps haven't resolved the issue,
or if :command:`workshop list` still shows remnants,
the most aggressive cleanup method is to completely purge the |ws_markup| snap.
This executes the snap's :samp:`remove` hook,
which is designed to clean up all associated data and resources.

.. warning::

   This is a highly destructive operation that removes all workshops
   for all users on the system. It should only be used as a last resort.
   You will need to reinstall |ws_markup| to use it again.


To purge the snap and all its data, run the following command:

.. code-block:: console

   $ sudo snap remove workshop --purge


This will remove all workshop configurations, containers, LXD profiles,
and storage pools managed by |ws_markup|.

After the command completes, you can reinstall the snap.


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

How-to guides:

- :ref:`how_debug_issues_workshops`
- :ref:`how_troubleshoot`


Reference:

- :ref:`ref_workshop_list`
- :ref:`ref_workshop_remove`