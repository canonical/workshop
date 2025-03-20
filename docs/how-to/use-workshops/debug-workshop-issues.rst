.. _how_debug_issues_workshops:

How to debug issues in workshops
================================

To trace the root cause
of a workshop misbehaving at :command:`workshop refresh` or any other action,
you can explore its underlying changes and tasks, pause on error,
list system-wide warnings and acknowledge false positives.


List workshop changes
---------------------

.. @artefact workshop changes

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
     - Run hook "setup-base" for "go" SDK (command exit code 1)
     "go" refresh aborted


To investigate the failure,
list the *changes* in the workshop to find the one that failed:

.. code-block:: console

   $ workshop changes

     ID  Status  Spawn                Ready                Summary
     ...
     81  Error   today at 12:20       today at 12:23       Refresh workshops "dev-volatile"


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

If no change ID is provided,
:command:`workshop tasks` inspects the most recent change
to the current project.


Wait on error
-------------

The :option:`!--wait-on-error` option in :command:`workshop refresh` and
:command:`workshop launch`
pauses the command when an error occurs;
instead of reverting the workshop to its previous state,
|ws_markup| will leave it as is for you to investigate:

.. code-block:: console

   $ workshop refresh --wait-on-error

     error: cannot perform the following tasks:
     - Run hook "setup-base" for "go" SDK (command exit code 1) 

     To proceed, resolve the issue and run "workshop refresh --continue go"
     To cancel and undo: "workshop refresh --abort go"
     To view more information: "workshop tasks 1"

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


Raw and Verbose
---------------

The :option:`!--verbose` or :option:`!--raw` flags can be used with 
:command:`launch` and :command:`refresh` 
to modify what is shown when running the commands. 

:option:`!--verbose` includes the output of any hooks currently being executed
inside a workshop underneath the currently running task. For example if an SDK
is running apt-get update in the `setup-base` hook, 
the output may look like the below:

.. code-block:: console
   
   $ worshop refresh --verbose
     Run hook "setup-base" for "example" SDK
     Get:48 http://archive.ubuntu.com/ubuntu noble-backports/universe amd64 c-n-f Metadata [1256 B]
     Get:49 http://archive.ubuntu.com/ubuntu noble-backports/restricted amd64 Components [216 B]
     Get:50 http://archive.ubuntu.com/ubuntu noble-backports/restricted amd64 c-n-f Metadata [116 B]
     Get:51 http://archive.ubuntu.com/ubuntu noble-backports/multiverse amd64 Components [212 B]
     Get:52 http://archive.ubuntu.com/ubuntu noble-backports/multiverse amd64 c-n-f Metadata [116 B]
     Fetched 31.3 MB in 7s (4405 kB/s)
     Reading package lists...


:option:`!--raw` includes the same information as :option:`!--verbose`, 
however renders it as simple text without any terminal effects. This is 
particularly useful for CI pipelines and other non-interactive environments:

.. code-block:: console
   
   $ workshop refresh --raw
     TASK: Create SDK state storage
     TASK: Run hook "save-state" for "example" SDK
     TASK: Remove "system" SDK profile
     TASK: Remove "example" SDK profile
     TASK: Stash previous "dev" workshop
     TASK: Create new "dev" workshop
     TASK: Mount project directory "/dev"
     TASK: Start "dev" workshop
     TASK: Install "system" SDK
     TASK: Install "example" SDK
     TASK: Run hook "setup-base" for "example" SDK
     Hit:1 http://archive.ubuntu.com/ubuntu noble InRelease
     Get:2 http://archive.ubuntu.com/ubuntu noble-updates InRelease [126 kB]
     Get:3 http://security.ubuntu.com/ubuntu noble-security InRelease [126 kB]
     Get:4 http://archive.ubuntu.com/ubuntu noble-backports InRelease [126 kB]


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
- :ref:`exp_sdk`
- :ref:`exp_workshop`


Reference:

- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_okay`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshop_warnings`
