.. _how_add_mounts:

.. meta::
   :description: How-to guide on adding mounts to a workshop with the mount
                 interface, covering host persistence, sharing existing host
                 directories, read-only exposure, and sharing data between SDKs.

How to add mounts to a workshop
===============================

.. @tests in tests/docs-how-to/add-mounts/task.yaml

.. @artefact mount interface

|ws_markup| exposes filesystem locations to a workshop
through the mount interface.
A plug declared on an SDK names a target directory inside the workshop;
|ws_markup| binds a source directory to it at run-time,
either from a path |ws_markup| allocates on the host
or from one inside the workshop.
There are five common scenarios worth discussing.


Persist workshop-internal files on the host
-------------------------------------------

To persist data that the workshop produces or uses outside the project directory
(tooling caches, user data, logs)
without picking and maintaining a host directory yourself,
add a plug under an SDK in :file:`workshop.yaml`
with :samp:`workshop-target` only:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 5-8

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: uv
       plugs:
         data:
           interface: mount
           workshop-target: /home/workshop/data


Refresh the workshop to bind the target:

.. @artefact workshop refresh

.. code-block:: console

   $ workshop refresh


|ws_markup| allocates a host directory for the plug at
:file:`~/.local/share/workshop/id/<PROJECT-ID>/<WORKSHOP>/mount/<SDK>/<PLUG>/`
and binds it to :file:`/home/workshop/data/` inside the workshop.
Files written there from the workshop survive
:command:`workshop start`, :command:`workshop stop`,
and :command:`workshop refresh`
because the data lives on the host.

This is the cheapest way to get persistence
when the workshop, not the user,
owns the lifecycle of the files.


Remount a host directory inside the workshop
--------------------------------------------

If the workshop needs to consume some ad-hoc data from the host,
declare the plug as before and then point it at a host path
with :command:`workshop remount`.

.. @artefact workshop remount

.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: uv
       plugs:
         shared:
           interface: mount
           workshop-target: /home/workshop/shared


Refresh the workshop, stop it, point the plug at the host path,
then start it again:

.. code-block:: console

   $ workshop refresh
   $ workshop stop dev
   $ workshop remount dev/uv:shared ~/datasets
   $ workshop start dev


The host path can be absolute or relative.
|ws_markup| only swaps a live mount atomically
when the new source is non-existent or empty.
For a populated source like :file:`~/datasets/`,
the workshop must be stopped first
to avoid corrupting in-flight reads or writes,
hence the :command:`workshop stop` and :command:`workshop start` above.

Inside the workshop,
:file:`~/datasets/` from the host
is now visible at :file:`/home/workshop/shared/`.

The :command:`workshop remount` command sets up a durable share;
use it for the host data the user owns
and wants the workshop to access across many sessions,
not just one.

Once a share source is set, :command:`workshop info` surfaces it
alongside the :samp:`workshop-target`:

.. @artefact workshop info

.. code-block:: console
   :emphasize-lines: 8

   $ workshop remount dev/uv:shared ~/datasets
   $ workshop info dev

     ...
     sdks:
       uv:
         mounts:
           shared:
             host-source:      /home/user/datasets
             workshop-target:  /home/workshop/shared
     ...


The override survives :command:`workshop refresh`,
so the share stays in place across SDK and base updates
without re-running :command:`workshop remount`.

More, :command:`workshop remove` does not delete the remounted host directory;
the data on disk is assumed to be the user's,
so |ws_markup| leaves it alone.
What *does* go away with the removed workshop is its *record* of the remount:
a fresh :command:`workshop launch` starts with the auto-allocated source
until you remount again.

To drop the override on demand without removing the workshop,
see :ref:`how_reset_remount`.


Expose a host directory read-only
---------------------------------

For shared reference data, configuration, or secrets
that the workshop should read but not modify,
add :samp:`read-only: true` to the plug:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 9

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: uv
       plugs:
         readonly:
           interface: mount
           workshop-target: /home/workshop/readonly
           read-only: true


Refresh the workshop, then point the plug at the host directory.
Stop the workshop first
if :file:`~/refdata/` already holds the reference data,
as in the previous scenario:

.. code-block:: console

   $ workshop refresh
   $ workshop stop dev
   $ workshop remount dev/uv:readonly ~/refdata
   $ workshop start dev


Writes to :file:`/home/workshop/readonly/` from inside the workshop
fail even with :samp:`sudo`.

The :samp:`mode`, :samp:`uid`, and :samp:`gid` plug attributes
control the permissions and ownership
of any directory that |ws_markup| creates on behalf of the plug.
Defaults are :samp:`1000:1000` for targets
under :file:`/home/workshop/`, :file:`/project/`, or :file:`/run/user/1000/`,
and root otherwise.
For the full ownership rules and override examples,
see :ref:`how_configure_mount`.

As with any remounted plug,
:command:`workshop remove` leaves the host directory in place.


Share a directory between SDKs
------------------------------

The mount interface also can connect SDKs in the same workshop
without going through the host.
The slot SDK declares :samp:`workshop-source`
to publish a directory inside the workshop;
the plug SDK consumes it at its :samp:`workshop-target`.

For instance,
the :samp:`uv` and :samp:`jupyter` SDKs ship this pattern out of the box:
:samp:`uv` exposes :file:`/home/workshop/uv-venv/` through a :samp:`venv` slot,
and :samp:`jupyter` consumes it with a :samp:`venv` plug at :file:`$SDK/venv/`.
Both declarations are already there,
so the workshop definition only grafts the pairing on.
Building the slot and the plug into SDKs you author
is covered by :ref:`how_share_content_between_sdks`.

The pairing has to be spelled out.
Without a :samp:`connections:` entry,
:samp:`jupyter:venv` auto-connects to :samp:`system:mount`,
receives a host directory,
and :samp:`uv:venv` stays unconsumed.

List both SDKs and wire them with an explicit :samp:`connections` entry:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 6-8

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: uv
     - name: jupyter
   connections:
     - plug: jupyter:venv
       slot: uv:venv


.. code-block:: console

   $ workshop refresh


After the refresh,
:samp:`jupyter` and :samp:`uv` share the same Python virtual environment,
and neither SDK is aware of the other.
Confirm the pairing:

.. code-block:: console

   $ workshop connections dev

     INTERFACE  PLUG              SLOT                 NOTES
     mount      dev/jupyter:venv  dev/uv:venv          -
     mount      dev/uv:cache      dev/system:mount     -
     tunnel     -                 dev/jupyter:jupyter  -


The :samp:`uv:cache` row shows the default that the entry overrides:
a mount plug nobody wires will land on :samp:`system:mount`.


.. _how_reset_remount:

Reset a remount
---------------

To drop a custom source set with :command:`workshop remount`
and return the plug to its auto-allocated host directory,
disconnect the plug with :option:`!--forget`,
which discards the source override,
then connect the plug to the system mount slot to re-establish it:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop disconnect dev/uv:shared --forget
   $ workshop connect dev/uv:shared :mount


|ws_markup| then re-binds :file:`/home/workshop/shared/`
to the auto-allocated directory under :file:`~/.local/share/workshop/`.

Note that :command:`workshop connections` now lists the plug as :samp:`manual`
in the :samp:`NOTES` column.
This manual connection persists across :command:`workshop refresh`
and :command:`workshop stop` plus :command:`workshop start` cycles,
but :command:`workshop restore` drops it
and returns the plug to its definition's auto-connect default.

.. note::

   For how manual connections behave across these operations,
   see :ref:`exp_workshop_connection_lifecycle`.


To revert connections and mounts wholesale,
use :command:`workshop restore`;
it also resets any source set with :command:`workshop remount`.
For a single plug, disconnect it with :option:`!--forget`
and connect it again, as shown above.


See also
--------

Explanation:

- :ref:`exp_best_dependencies`
- :ref:`exp_mount_interface`
- :ref:`exp_system_sdk`
- :ref:`exp_workshop_connection_lifecycle`


How-to guides:

- :ref:`how_configure_mount`
- :ref:`how_share_content_between_sdks`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_remount`
- :ref:`ref_workshop_restore`
