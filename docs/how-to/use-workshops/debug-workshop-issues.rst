.. _how_debug_issues_workshops:

How to debug issues in workshops
================================

To trace the root cause
of a workshop misbehaving at :command:`workshop refresh` or any other action,
you can explore its underlying changes and tasks, stop on error,
list system-wide warnings and acknowledge false positives.


List workshop changes
---------------------

.. @artefact workshop changes

Consider a workshop named :samp:`golang-volatile`,
which uses an unstable SDK
from the :samp:`latest/edge` channel:

.. code-block:: yaml
   :caption: workshop.yaml

   name: golang-volatile
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


To investigate the failure,
list the *changes* in the workshop to find the one that failed:

.. code-block:: console

   $ workshop changes

     ID  Status  Spawn                Ready                Summary
     ...
     81  Error   today at 12:20       today at 12:23       Refresh workshops "golang-volatile"


List tasks in a change
----------------------

When you have found the problematic change,
list its *tasks* to see the cause:

.. @artefact workshop tasks

.. code-block:: console

   $ workshop tasks 81

     ID    Status  Spawn                Ready                Summary
     ...
     1392  Error   today at 12:17       today at 12:18       Run hook "setup-base" for "go" SDK

     ......................................................................
     Run hook "save-state" for "go" SDK

     2023-07-24T12:17:37+12:00 INFO latest/beta save-state: preserving ~/.config/pretrained-config.conf
     ......................................................................
     Run hook "setup-base" for "go" SDK
     ...
     Traceback (most recent call last):
         File "<string>", line 1, in <module>
         File "/home/user/.local/lib/python3.9/site-packages/tensorrt/__init__.py", line 36, in <module>
             from .tensorrt import *
     ModuleNotFoundError: No module named 'tensorrt.tensorrt'

The SDK-specific reason can be addressed individually.


Wait on error
-------------

The :option:`!--wait-on-error` option in :command:`workshop refresh` and
:command:`workshop launch`
pauses the refresh when an error occurs;
instead of reverting the workshop to its previous state,
|ws_markup| will leave it as is for you to investigate:

.. code-block:: console

   $ workshop refresh --wait-on-error

     YYYY-MM-DDT00:00:00 ERROR command exit code 1
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
       golang-volatile/go:mod-cache mount is broken: /home/user/mod-cache does not exist


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
- :ref:`exp_sdk`
- :ref:`exp_workshop`


Reference:

- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_okay`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshop_warnings`
