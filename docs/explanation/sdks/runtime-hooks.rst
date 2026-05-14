.. _exp_sdk_hooks:

.. meta::
   :description: Runtime hooks let an SDK extend the workshop lifecycle at
                 well-defined points, with a precise execution contract for
                 privileges, environment, working directory, and ordering.

Runtime hooks
=============

.. @artefact SDK
.. @artefact SDK hook
.. @artefact restore-state
.. @artefact save-state
.. @artefact setup-base
.. @artefact setup-project
.. @artefact check-health

A *hook* is a bash script
that an SDK ships under its :file:`hooks/` directory.
|ws_markup| runs the script at a specific point in the workshop lifecycle
to give the SDK a chance to set up its environment,
report its health,
or persist data across a refresh.
Without hooks,
an SDK is a passive bundle of files;
hooks are how it participates in the running workshop.

|sdk_markup| enumerates an SDK's hooks automatically when packing it,
so they do not need to be listed in the
:ref:`SDK definition <exp_sdk_definition>`.


The five hooks
--------------

|ws_markup| recognises five hooks,
each one answering a different question about the SDK's relationship
to the workshop.

- :samp:`setup-base`:
  Runs once per workshop change (launch or refresh)
  after the workshop process is up
  but before the project directory is mounted
  and before plugs and slots are connected.
  This is where an SDK does the system-wide preparation
  it needs before it can serve plugs:
  installing :file:`/etc/profile.d/` snippets,
  wiring shell completions,
  registering alternatives,
  or anything else that requires root and that the workshop's other SDKs
  may want to rely on.

- :samp:`setup-project`:
  Runs after the project directory is mounted
  and after auto-connect has finished,
  but before the workshop is set to *Ready*.
  This is per-project initialization
  from the perspective of the workshop user:
  touching files in the user's home,
  priming a project-local cache,
  creating a project-relative workspace
  from data that the SDK provides.

- :samp:`check-health`:
  Gives the SDK a chance to report
  whether it is functioning inside this particular workshop.
  It is intended to be fast,
  and the workshop only waits for it briefly
  before deciding the SDK has not reported back.
  See :ref:`exp_workshopctl_health` below
  for how the result feeds into the workshop status.

- :samp:`save-state`
  Runs during a refresh,
  before the old SDK revision is taken down.
  It exists because some SDK state
  does not live behind a connectable plug
  and would otherwise be lost when the workshop swaps revisions.

- :samp:`restore-state`:
  Runs during the same refresh,
  after the new SDK revision has set up its project,
  and reads back what the old revision saved.
  See :ref:`exp_sdk_state` for the persistence model.


Execution contract
------------------

Every hook runs as a :program:`bash` script
with the :samp:`errexit` and :samp:`pipefail` options set,
which means a non-zero exit code
from any command,
or any pipe stage,
ends the hook and surfaces the failure
to the workshop change that triggered it.
When :option:`!--verbose` is passed to
:command:`workshop launch` or :command:`workshop refresh`,
the :samp:`xtrace` option is also set,
making each command in the script visible in the change output.

The runner provides the :envvar:`$SDK` directory variable to every hook;
beyond that,
some hooks see additional variables in scope.
The privilege, working directory, and extra variables
differ by hook
(:samp:`None` in the table below means *no extras beyond* :envvar:`$SDK`,
not no environment at all):

.. list-table::
   :header-rows: 1
   :widths: 18 28 18 36

   * - Hook
     - Working directory
     - Runs as
     - Extra environment

   * - :samp:`setup-base`
     - The SDK's :file:`hooks/` directory
     - :samp:`root`
     - None

   * - :samp:`setup-project`
     - :file:`/project/`
     - The :samp:`workshop` user
     - :envvar:`$HOME`, :envvar:`$XDG_RUNTIME_DIR`,
       :envvar:`$DBUS_SESSION_BUS_ADDRESS`

   * - :samp:`check-health`
     - The SDK's :file:`hooks/` directory
     - :samp:`root`
     - None

   * - :samp:`save-state`
     - The SDK's :file:`hooks/` directory
     - :samp:`root`
     - :envvar:`$SDK_STATE_DIR`

   * - :samp:`restore-state`
     - The SDK's :file:`hooks/` directory
     - :samp:`root`
     - :envvar:`$SDK_STATE_DIR`


For the precise launch and refresh stages at which each hook fires,
see the :ref:`hook reference <ref_sdk_hooks>`.


Ordering across SDKs
--------------------

When several SDKs in a workshop define the same hook,
|ws_markup| runs them sequentially,
not in parallel,
and waits for each hook to finish
before starting the next.

The order is fixed:
the :ref:`system SDK <exp_system_sdk>` first,
then the user-listed SDKs in the order they appear
in the :ref:`workshop definition <exp_workshop_definition>`,
then the sketch SDK if present.
There is no dependency resolution between SDKs;
the listing order is the contract.
If an SDK relies on another SDK's :samp:`setup-base` having finished,
list it later in the workshop definition.


.. _exp_workshopctl:
.. _exp_workshopctl_health:

Talking back with workshopctl
-----------------------------

.. @artefact workshopctl
.. @artefact workshop status

From inside a hook,
the SDK can call :program:`workshopctl`
to interact with the workshop daemon.

The most common use is :command:`workshopctl set-health` from
:samp:`check-health`,
which sets the SDK's health to :samp:`okay`, :samp:`waiting`, or :samp:`error`.
The workshop's overall :ref:`status <exp_workshop_status>`
is derived from the union of these results:

- The hook sets health to :samp:`okay` and exits with code zero:
  the SDK is *Ready*.

- The hook sets health to :samp:`waiting`:
  |ws_markup| retries :samp:`check-health` once per second.
  After ten consecutive retries,
  or if five seconds pass without :samp:`set-health` being invoked at all,
  the SDK is moved to *Error*.

- The hook exits with a non-zero code,
  or sets health to :samp:`error` explicitly:
  the SDK is *Error*.


.. _exp_sdk_state:

State persistence across refresh
--------------------------------

.. @artefact SDK state

A workshop refresh swaps an SDK from one revision to another,
which means anything the SDK had in the old revision's filesystem
disappears unless it lives behind a mount plug.
For state that should survive a refresh
but is not exposed as a connectable resource,
:samp:`save-state` and :samp:`restore-state`
are the right place to put it.

The runner provides :envvar:`$SDK_STATE_DIR` to both hooks,
pointing at a directory that persists across the refresh.
:samp:`save-state` runs from the *old* SDK revision
before the swap,
and writes whatever needs to outlive the revision into that directory.
:samp:`restore-state` runs from the *new* SDK revision
after the swap and after :samp:`setup-project` has run for every SDK,
and reads from the same directory to reapply what was saved.

:envvar:`$SDK_STATE_DIR` is only in scope for these two hooks;
the :samp:`workshop` user, the SDK's own running processes,
and the other hooks cannot see it.
That keeps the state directory out of reach
of anything that should not be writing to it.


See also
--------

Explanation:

- :ref:`exp_arch_runtime_behavior`
- :ref:`exp_sdk_best_practices`
- :ref:`exp_sdk_concepts`
- :ref:`exp_sdks`


Reference:

- :ref:`ref_sdk_hooks`
- :ref:`ref_sdk_state`
- :ref:`ref_workshopctl__cli`


Tutorial:

- :ref:`tut_craft_sdks`
