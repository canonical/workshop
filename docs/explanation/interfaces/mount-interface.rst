.. _exp_mount_interface:

Mount interface
===============

The mount interface securely exposes file system locations
on the host (via the :ref:`system SDK <exp_system_sdk>` only) or in the workshop
by mounting them inside the workshop.

By using the interface,
the SDK publisher enables the use of the mount mechanism in the workshop;
a host location also allows persisting data outside the workshop.

.. @artefact host data source

The interface defines a target directory inside the workshop,
to which a source directory is mounted at run-time.
Typically, it would provide resources to be consumed by the SDK,
accumulated over time or created
when the :command:`workshop launch` or :command:`workshop refresh` commands run:

- The slot is the provider,
  indicating that any data placed in its source directory
  can be used by a workshop via a plug.

- The plug is the consumer,
  indicating that the data will be available at the target directory,
  where the SDK or the user presumably expects it.


.. _exp_mount_plug:

Mount interface plug
--------------------

An essential element here is the mount interface plug,
which is declared in the SDK definition.

A basic structure would include the name of the plug itself,
the interface (:samp:`mount`)
the intended target path inside the workshop (:samp:`workshop-target`)
and, optionally, whether the mount should be read-only (:samp:`read-only`).

Defining the plug in an SDK designates the target directory inside the workshop;
a directory on the host system that |ws_markup| will create at run-time
will be mounted to it.

This allows the workshops using this SDK to use the host directory
(which |ws_markup| allocates automatically and doesn't expose otherwise)
to persist the files placed there from inside the workshop
in the host file system when the workshop stops.


.. _exp_mount_slot:

Mount interface slot
--------------------

To let SDKs in a workshop access the host file system,
|ws_markup| provides a mount interface slot
that multiple mount interface plugs can access.

When the SDK is installed at run-time during launch and refresh operations,
|ws_markup| checks the following for each plug that targets the slot:

- The plug can be installed.

- The plug can be auto-connected
  (for :samp:`mount`, it's a yes).

- The :samp:`workshop-target` directory already exists in the workshop.


If the plug passes the checks, it is connected.


Connection
----------

The interface is connected automatically at launch or refresh
if the plug can be matched to the slot by its name
or via a :samp:`connections` entry in the :ref:`definition <exp_workshop_definition>`,
both subject to |ws_markup|'s
:ref:`validation rules <exp_interfaces_validation>`.

Establishing a connection means the source directory created by |ws_markup|
is mounted to the target directory inside the workshop.
The source directory can be created:

- At a designated path inside the workshop,
  which needs a slot with :samp:`workshop-source` set

- At an internal location on the host,
  which |ws_markup| assigns if no slot is set explicitly


If the directory is created on the host,
its contents are preserved between operations such as
:command:`workshop refresh`, :command:`workshop start`,
and :command:`workshop stop`.

After the workshop has started,
the :command:`workshop connect` and :command:`workshop disconnect` commands
can be used to manage the connection manually.

To check if the interface is connected:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections --all

     Interface  Plug                Slot    Notes
     ...
     mount      ws/mount-sdk:cache  :cache  manual


This means a source directory is mounted to the target:

.. @artefact workshop info

.. code-block:: console
   :emphasize-lines: 13

   $ workshop info ws

     name:     ws
     base:     ubuntu@22.04
     project:  /home/user/workshops/ws
     status:   ready
     notes:    -
     sdks:
       mount-sdk:
         tracking:   latest/edge
         installed:  2022-03-04  (1)
         mounts:
           cache:
             host-source:      .../8584e571/mount/ws_mount-sdk_cache.sdk
             workshop-target:  /home/workshop/.local/cache


Here, the source is set to an internal location (:samp:`...`)
that |ws_markup| maintains on the host file system;
the SDKs can't set host locations explicitly for security reasons,
but there's a way to do it manually.


Remount
-------

The :command:`workshop remount` command sets a new source directory on the host
for the target directory inside the workshop:

.. @artefact workshop remount

.. code-block:: console

   $ workshop remount ws/mount-sdk:cache ~/.local/cache/


First, the remount operation is attempted atomically;
this usually succeeds if the new source is either a non-existent directory
or an empty directory on the same file system as the current source.
Otherwise, the remount only occurs if the workshop has been stopped earlier,
which prevents data corruption.

To reset a remounted plug to its default source location,
use :samp:`workshop disconnect` with the :option:`!--forget` option,
then refresh the workshop:

.. @artefact workshop disconnect
.. @artefact workshop refresh

.. code-block:: console

  $ workshop disconnect ws/mount-sdk:cache --forget
  $ workshop refresh ws


See also
--------

Explanation:

- :ref:`exp_interfaces`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdk_definition`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_remount`
- :ref:`ref_workshop_start`
- :ref:`ref_workshop_stop`
