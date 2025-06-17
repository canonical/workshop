.. _how_debug_issues_workshops:

How to debug issues in workshops
================================

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
       channel: latest/edge


Suppose something goes wrong during :command:`workshop refresh`:

.. code-block:: console

   $ workshop refresh

     Error: cannot perform the following tasks:
     - Run hook "setup-base" for "go" SDK (command failed with an error code (1))
     Refresh aborted


To see the *tasks*, or individual actions,
during the latest *change*, which is essentially a major workshop update,
run :command:`workshop tasks` without arguments:

.. code-block:: console

   $ workshop tasks

     ID    Status  Spawn           Ready           Summary
     ...


If that didn't help,
investigate the failure further
by listing all *changes* in the workshop to find the one that failed:

.. code-block:: console

   $ workshop changes

     ID  Status  Spawn           Ready           Summary
     ...
     81  Error   today at 12:20  today at 12:21  Refresh workshops "dev-volatile"


When you have found the problematic change,
list its *tasks* to see the cause,
this time supplying the change ID as the argument:

.. code-block:: console

   $ workshop tasks 81

     ID    Status  Spawn           Ready           Summary
     1390  Undone  today at 12:20  today at 12:21  Create SDK state storage
     1391  Done    today at 12:20  today at 12:21  Run hook "save-state" for "go" SDK
     1392  Done    today at 12:20  today at 12:21  Disconnect interfaces of "go" SDK
     1393  Done    today at 12:20  today at 12:21  Disconnect interfaces of "system" SDK
     1394  Undone  today at 12:20  today at 12:21  Unregister "go" SDK plugs and slots
     1395  Undone  today at 12:20  today at 12:21  Stash previous "dev-volatile" workshop
     1396  Undone  today at 12:20  today at 12:21  Restore "dev-volatile" workshop from "system" snapshot
     1397  Undone  today at 12:20  today at 12:21  Start "dev-volatile" workshop
     1398  Undone  today at 12:20  today at 12:21  Install "go" SDK
     1399  Undone  today at 12:20  today at 12:21  Register "go" SDK plugs and slots
     1400  Error   today at 12:20  today at 12:21  Run hook "setup-base" for "go" SDK
     1401  Hold    today at 12:20  today at 12:21  Auto-connect interfaces of "system" SDK
     1402  Hold    today at 12:20  today at 12:21  Auto-connect interfaces of "go" SDK
     1403  Hold    today at 12:20  today at 12:21  Run hook "restore-state" for "go" SDK
     1404  Hold    today at 12:20  today at 12:21  Run hook "check-health" for "system" SDK
     1405  Hold    today at 12:20  today at 12:21  Run hook "check-health" for "go" SDK
     1406  Hold    today at 12:20  today at 12:21  Remove SDK state storage
     1407  Hold    today at 12:20  today at 12:21  Remove "dev-volatile" workshop from stash
     1408  Done    today at 12:20  today at 12:21  Remove "go" SDK profile
     1409  Done    today at 12:20  today at 12:21  Remove "system" SDK profile

     ......................................................................
     Run hook "save-state" for "go" SDK

     2023-07-24T12:21:37 INFO GOBIN='/home/workshop/.local/bin'

     ......................................................................
     Run hook "setup-base" for "go" SDK

     2023-07-24T12:21:37 ERROR error: cannot install "go": cannot get nonce from store: persistent network
     2023-07-24T12:21:37 ERROR        error: Post "https://api.snapcraft.io/api/v1/snaps/auth/nonces": dial
     2023-07-24T12:21:37 ERROR        tcp: lookup api.snapcraft.io: Temporary failure in name resolution (exit code: 1)


The SDK-specific reason can be addressed individually.


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

To help determine what went wrong, use the :command:`workshop changes` and
:command:`workshop tasks` commands discussed above.

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


List and suppress warnings
--------------------------

|ws_markup| occasionally encounters non-blocking or transient problems,
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
- :ref:`ref_workshop_okay`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshop_warnings`
