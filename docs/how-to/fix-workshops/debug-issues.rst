.. _how_debug_issues_workshops:

.. meta::
   :description: How-to guide on debugging workshop issues, covering tasks like tracing
                 changes, pausing on errors, and managing system-wide warnings.

How to debug issues in workshops
================================

.. @tests in tests/docs-how-to/debug-issues/task.yaml

To trace the root cause
of a workshop misbehaving at :command:`workshop refresh` or any other action,
you can explore its underlying changes and tasks, pause on error,
list system-wide warnings, and acknowledge false positives.


List tasks and changes
----------------------

.. @artefact workshop changes
.. @artefact workshop tasks

Consider a workshop named :samp:`dev-volatile`,
which uses an unstable SDK
from the :samp:`latest/edge` channel:

.. code-block:: yaml
   :caption: workshop.yaml

   name: dev-volatile
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: edge


Suppose something goes wrong during :command:`workshop refresh`:

.. code-block:: console

   $ workshop refresh

     Error: cannot perform the following tasks:
     - Run hook "setup-base" for "go" SDK (command failed with an error code (1))
     Refresh aborted


To show more details, try :command:`workshop refresh --verbose`;
you can also use :option:`!--verbose` with :command:`workshop launch`.

To see the *tasks*, or individual actions,
during the latest *change*, which is essentially a major workshop update,
run :command:`workshop tasks` without arguments:

.. code-block:: console

   $ workshop tasks

     STATUS   DURATION  SUMMARY
     ...


If that didn't help,
investigate the failure further
by listing all *changes* in the workshop to find the one that failed:

.. code-block:: console

   $ workshop changes

     ID  STATUS  SPAWN           READY           SUMMARY
     ...
     81  Error   today at 12:20  today at 12:21  Refresh workshops "dev-volatile"


When you have found the problematic change,
list its *tasks* to see the cause,
this time supplying the change ID as the argument:

.. code-block:: console

   $ workshop tasks 81

     STATUS    DURATION  SUMMARY
     Done          59ms  Retrieve "go" SDK from channel "latest/edge"
     Undone        42ms  Create SDK state storage
     Done          28ms  Run hook "save-state" for "go" SDK
     Done          31ms  Disconnect interfaces of "go" SDK
     Done          29ms  Disconnect interfaces of "system" SDK
     Undone        35ms  Uninstall "go" SDK
     Undone        48ms  Stash previous "dev-volatile" workshop
     Undone        52ms  Restore "dev-volatile" workshop from "system" snapshot
     Undone        41ms  Start "dev-volatile" workshop
     Undone        67ms  Install "go" SDK
     Error      1m12.5s  Run hook "setup-base" for "go" SDK
     Hold             -  Snapshot "go" SDK installation
     Hold             -  Mount project directory
     Hold             -  Resolve relations between interfaces of "dev-volatile" workshop
     Hold             -  Auto-connect interfaces of "go" SDK
     Hold             -  Run hook "setup-project" for "go" SDK
     Hold             -  Run hook "restore-state" for "go" SDK
     Hold             -  Run hook "check-health" for "go" SDK
     Hold             -  Remove SDK state storage
     Hold             -  Remove "dev-volatile" workshop from stash
     Done          24ms  Remove "go" SDK profile

     ......................................................................
     Run hook "save-state" for "go" SDK

     2023-07-24T12:21:37 INFO GOBIN='/home/workshop/.local/bin'

     ......................................................................
     Run hook "setup-base" for "go" SDK

     2023-07-24T12:21:37 ERROR error: cannot install "go": cannot get nonce from store: persistent network
     2023-07-24T12:21:37 ERROR        error: Post "https://api.snapcraft.io/api/v1/snaps/auth/nonces": dial
     2023-07-24T12:21:37 ERROR        tcp: lookup api.snapcraft.io: Temporary failure in name resolution (exit code: 1)


The SDK-specific reason can be addressed individually.


.. _how_debug_wait_on_error:

Wait on error
-------------

.. @artefact workshop launch
.. @artefact workshop refresh

The :option:`!--wait-on-error` option in :command:`workshop refresh` and
:command:`workshop launch`
pauses the refresh when an error occurs;
instead of reverting the workshop to its previous state,
|ws_markup| will leave it as is for you to investigate:

.. code-block:: console

   $ workshop refresh --wait-on-error

     2023-07-24T12:22:42+12:00 ERROR command exit code 1
     error: cannot refresh; fix the errors reported,
     then run "workshop refresh --continue blank".
     To abort and revert, run "workshop refresh --abort blank"

To help determine what went wrong, use the :command:`workshop changes` and
:command:`workshop tasks` commands discussed above.

Next, you can shell into the workshop to debug and possibly fix it:

.. @artefact workshop shell

.. code-block:: console

   $ workshop shell


On success, you can resume the refresh process:

.. code-block:: console

   $ workshop refresh --continue


Otherwise, undo the changes with the :option:`!--abort` option:

.. code-block:: console

   $ workshop refresh --abort


The effect will be the same as if you hadn't used :option:`!--wait-on-error`:
the workshop will revert to its previous state.


Isolate problematic SDKs
------------------------

When a workshop uses multiple SDKs
and has issues during :command:`workshop refresh` or :command:`workshop launch`,
it can be difficult to determine which SDK is causing the problem.

Start by testing each SDK in isolation before combining them;
this helps narrow down compatibility issues,
integration problems,
or SDK-specific bugs.

If the workshop fails only when multiple SDKs are used together,
the issue may stem from interactions between them.
To isolate the culprit,
comment out SDKs one by one in the workshop definition
and refresh the workshop after each change.

When the issue reappears,
the cause is likely the SDK you just re-enabled,
or its interaction with other SDKs.
Investigate it using the :command:`workshop tasks` command
to view detailed error information.


SDK-installed software versions
-------------------------------

Components installed via SDKs
cannot be updated using their regular mechanisms.
SDKs are mounted read-only inside workshops,
so regular update commands won't affect the SDK-provided files,
likely failing instead.

SDK definitions identify possible base systems
and are usually versioned after the software they install;
different SDK versions may be published via different channels.

To update any components provided by an SDK,
refresh the workshop with :command:`workshop refresh`.
This pulls the latest version of each SDK from its configured channel
and installs the updated SDK components.

To switch to a different version,
update the channel in the workshop definition,
then refresh the workshop to apply the change.


List and suppress warnings
--------------------------

|ws_markup| occasionally encounters nonblocking or transient problems,
such as broken mount points.
These are registered as *warnings* in a system-wide log,
which can be accessed with :command:`workshop warnings`:

.. @artefact workshop warnings

.. code-block:: console

   $ workshop warnings

     last-occurrence:  4 days ago, at 17:52 GMT
     warning: |
       dev-volatile/go:mod-cache mount is broken: /home/user/mod-cache does not exist


Multiple warnings about the same problem aren't stacked;
only their first and last occurrences are logged.
You can suppress listed warnings with :command:`workshop okay` to ignore them:

.. @artefact workshop okay

.. code-block:: console

   $ workshop okay


See also
--------

Explanation:

- :ref:`exp_changes_tasks`
- :ref:`exp_sdks`
- :ref:`exp_workshop`


Reference:

- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_okay`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshop_warnings`
