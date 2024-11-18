.. _ref_workshop_remount:

workshop remount
----------------

Mount a new source location to the content interface plug's target

Synopsis
~~~~~~~~


This command mounts a new source location on the host to the target directory
of the specified content interface plug, qualified by the SDK name.
Specifically, it does the following:

- Attempts the mount operation atomically;
  this normally succeeds if the new source is either a non-existing directory
  or an empty directory on the same file system as the current source

- Otherwise, performs the mount operation only if the workshop is *Stopped*
  to prevent data corruption


Notes:

- To stop the workshop, use 'workshop stop'

- 'workshop info' lists any mounted content interface plugs for the workshop

- 'workshop refresh' mounts the last source set by 'workshop remount', if any

- During 'workshop remove', non-default sources set by 'workshop remount'
  aren't removed


.. code-block:: console

   workshop remount <WORKSHOP>/<SDK>:<PLUG> <SOURCE> [flags]

Options
~~~~~~~
--no-wait

   Return the change ID, don't wait for the operation to finish


