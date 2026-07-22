.. _how_purge:

.. meta::
   :description: How-to guide on purging malfunctioning workshops, covering steps to
                 remove containers, metadata, and files thoroughly.

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

- Tried :command:`workshop remove <WORKSHOP>`
  or confirmed that the standard removal flow cannot be used.

- Backed up any workshop data you need to keep;
  manual cleanup can permanently delete containers, metadata, and files.

- Access to :command:`sudo` and :command:`lxc`
  for manual LXD cleanup.


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
first try :ref:`recreating its project directory <how_purge_orphaned>`,
which restores the standard removal flow without touching LXD.
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


Commands that resolve the project by its pathname,
including :command:`workshop remove` with the :option:`!--project` option,
no longer work for orphaned workshops:

.. code-block:: console

   $ workshop remove --project ~/projects/nimble nimble

     error: cannot create or load project at "/home/user/projects/nimble": lstat /home/user/projects/nimble: no such file or directory


However, the daemon keeps tracking the project's original location,
so you can restore the standard removal flow
by recreating the directory:

#. Recreate the directory at the same absolute path;
   it can remain empty:

   .. code-block:: console

      $ mkdir -p ~/projects/nimble


#. Remove the workshop,
   pointing at the recreated directory:

   .. code-block:: console

      $ workshop remove --project ~/projects/nimble nimble


#. Verify the removal and delete the recreated directory:

   .. code-block:: console

      $ workshop list --global
      $ rm -r ~/projects/nimble


.. note::

   Removal isn't the only option:
   running any workshop command against the recreated directory
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

- :ref:`how_debug_issues_workshops`
- :ref:`how_troubleshoot`


Reference:

- :ref:`ref_workshop_list`
- :ref:`ref_workshop_remove`
