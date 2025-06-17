.. _ref_workshop_internals:

Workshop internals
==================

This topic speaks about what goes into building and running a workshop.


What a workshop is
------------------

.. @artefact workshop (container)
.. @artefact workshopd

A workshop is an container-based environment intended for a single user,
fully described in a :ref:`definition file <ref_workshop_definition>`.

|ws_markup| currently uses LXD as its container engine,
communicating to it via the socket-activated :program:`workshopd` backend,
but that is an implementation detail and potential subject to change.

When you launch a workshop, you create and start a container with a base image.
The image is downloaded from the container engine server and cached locally.

.. @artefact SDK
.. @artefact SDK Store

On top of the base image, one or more SDKs are usually applied,
which provide tools for development and runtime tasks.
SDKs are downloaded from the SDK Store that is specific to |ws_markup|.

After the SDKs are installed, their :samp:`setup-base` hooks run
in the order the SDK are listed;
this serves to customize the workshop and prepare the SDKs for use.

The host user running |ws_markup| is mapped to the default workshop user,
named :samp:`workshop` in the container,
with :samp:`uid=1000` and :samp:`gid=1000`.
This ensures that files and directories changed by the :samp:`workshop` user
inside the container map back to the same user on the host,
preserving consistent ownership and permissions.


User sessions and ID mapping
----------------------------

.. @artefact workshop exec
.. @artefact workshop run
.. @artefact workshop shell

By default, all commands that execute something inside a workshop
(:command:`exec`, :command:`run`, :command:`shell`)
run login shells as :samp:`uid=1000` and :samp:`gid=1000`,
unless you explicitly change the IDs.
This ensures files created inside the container map back to the host user.

By default, :envvar:`$XDG_RUNTIME_DIR` is set to :samp:`/run/user/1000/`,
and a user-scoped session bus address is created at :samp:`/run/user/1000/bus`.
Using other IDs can break sessions.

Finally, it's worth mentioning that having an active session
doesn't prevent :command:`refresh` or any other change from running.
However, you may need to restart the session to see the effect.


How workshops are built
-----------------------

.. @artefact workshop launch

At the container launch, |ws_markup| does the following:

- Reads the workshop definition to see which base image and SDKs are needed.

- Fetches the required images and SDKs from the image server and the Store,
  if they aren't already cached locally.

- Reads definitions of local SDKs,
  such as the sketch SDK,
  and copies them to a read-only location.

- Spins up the container with mapped user and group IDs,
  configures basic devices like the root disk and network bridge.

- Installs the SDKs, then runs their setup hooks in the container.

- Sets up interface plugs and slots defined by the workshop and its SDKs
  for extra devices and capabilities.

- Maps the project directory on the host to :file:`/project` in the container;
  this allows to transparently work on the host-based files
  using the tools provided by the SDKs.

- Starts the container so it is ready for development tasks.


Thus, the container is built, or launched in |ws_markup| terms.
All subsequent start and stop operations affect an already built container;
any rebuilds occur only with a refresh.

After a successful start, you have a running container named for your workshop,
accessible via the regular container engine capabilities.
From a user's perspective, it behaves like a custom Ubuntu environment
tailored to a specific project.


How workshops are updated
-------------------------

.. @artefact workshop refresh

|ws_markup| has a refresh manager that tracks all updates in a workshop
(changes to the base image, adding or removing SDKs, updates to the definition)
and builds an update plan to decide whether a refresh is needed.

Some changes don't cause a refresh by their nature;
for example, updated scripts in the definition are copied inside the workshop.
Larger ones, like switching base images or changing the SDK layout,
trigger the refresh mechanism:

- A snapshot of the current container is stashed as a fallback.
- A new container is built based on the updated setup.
- If the build succeeds, the old container is dropped.
  If it fails, the system reverts to the previous snapshot.
- After a successful refresh, the stash is cleaned up.

This prevents broken states during major refreshes.


ZFS storage and pool size
-------------------------

|ws_markup| stores its containers and data on a ZFS pool by default.
This approach consolidates container images, :program:`apt` caches, SDKs
and other workshop content under a single system.

If you need more space or different performance,
you can resize or tune the ZFS pool (it's named :samp:`workshop`),
using the :command:`lxc storage` command
as suggested in this `LXD documentation section
<https://documentation.ubuntu.com/lxd/latest/howto/storage_pools/>`_.
However, day-to-day usage requires little manual intervention.

.. attention::

   Don't use the default ZFS utilities to alter the LXD-managed ZFS pool,
   as this may cause issues with LXD.


Additionally, |ws_markup| ensures the LXD storage pool itself is at least 5 GiB.
Otherwise, LXD allocates 20% of the available disk space by default.
If the total disk space is under 14 GiB,
this would result in a pool size of only about 2 GiB per workshop,
making it more likely to run out of space.


Details of the :program:`apt` cache
-----------------------------------

To speed up repeated software installations,
each workshop maintains a package cache at :file:`/var/cache/apt/archives`.
Because the container is single-user,
only the mapped user and root can access that cache.

When a workshop is removed, the :program:`apt` cache directory is removed as well.


Interfaces
----------

.. @artefact interface

As a reminder, |ws_markup| enforces resource access with plugs and slots.
A plug requests a resource (for example, the GPU or a socket),
and a slot provides it.

However, only the default workshop user
can access the host resources that interfaces expose inside the container.
Other users may exist (for example, those hard-coded in an SDK),
but they do not have that access and are not intended to.

In particular, this includes interface-based SSH or desktop sessions,
which are also limited to the default workshop user.


See also
--------

Explanation:

- :ref:`exp_workshop_concepts`
- :ref:`exp_workshop_status`


Reference:

- :ref:`ref_workshop_cli`
- :ref:`ref_workshop_status`
