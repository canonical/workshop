.. _tutorial:

Tutorial
========

This is a practical introduction
that takes you on a tour
of the essential |project_markup| activities.

Here, you will put into practice all major steps
in the life cycle of a *workshop*,
from :ref:`defining <tut_define>`, :ref:`launching <tut_launch>`
and :ref:`refreshing <tut_refresh>` it
to :ref:`executing commands <tut_exec>` in the workshop environment
and finally :ref:`deleting <tut_remove>` it.
The actions you're about to take
comprise the majority of your daily needs with |project_markup|.

Refer to the
:ref:`explanation <exp_index>`
if you need a more descriptive overview.
For comprehensive details, explore the
:ref:`reference <ref_index>`.
Finally, see the
:ref:`how-to guides <howto_index>`
if you're looking for advanced practical steps.


.. attention::

   One technical detail before you start:
   currently, |project_markup| supports only :samp:`amd64`.


Install |project_markup|
------------------------

Check the prerequisites,
build and install |project_markup|,
then make sure it runs.


Check prerequisites
~~~~~~~~~~~~~~~~~~~

|project_markup| requires
`LXD <https://canonical.com/lxd>`_
for low-level operation,
using its
`REST API <https://documentation.ubuntu.com/lxd/en/latest/restapi_landing/>`_
to configure individual *workshops*.

.. note::

   This means you can use regular :command:`lxc` commands
   to monitor |project_markup| activities, for example:

   .. code-block:: console

      $ lxc list --all-projects


First, install and
`initialise <https://documentation.ubuntu.com/lxd/en/latest/howto/initialize/>`_
LXD.
It's available as a snap:

.. tabs::
   .. group-tab:: Using :program:`snap`

      .. code-block:: console

         $ sudo snap install lxd
         $ sudo lxd init --auto


   .. group-tab:: Other ways

      See the available installation options in
      `LXD documentation
      <https://documentation.ubuntu.com/lxd/en/latest/installing/>`_.


Next, ensure the
`LXD daemon
<https://documentation.ubuntu.com/lxd/en/latest/explanation/lxd_lxc/#lxd-daemon>`_
is enabled and running:

.. tabs::
   .. group-tab:: Using :program:`snap`

      .. code-block:: console

         $ sudo snap start --enable lxd.daemon
         $ snap services lxd.daemon

   .. group-tab:: Other ways

      Refer to
      `LXD documentation
      <https://documentation.ubuntu.com/lxd/en/latest/installing/>`_
      and your distribution's manuals for guidance.


Install
~~~~~~~

Build the :program:`workshop` snap
from the |project_markup| source code on
`GitHub`_:

.. code-block:: console

   $ git clone git@github.com:canonical/workshop.git  # or git clone https://github.com/canonical/workshop.git
   $ cd workshop
   $ sudo snap install snapcraft --classic
   $ snapcraft clean && snapcraft

.. tip::

   In case of :program:`lxd`-related issues with :program:`snapcraft`,
   ensure you're a member of the :samp:`lxd` group:
   
   .. code-block:: console
      
      $ id -nG <USERNAME>
      $ sudo adduser <USERNAME> lxd

Install the resulting :file:`.snap` file,
for example:

.. code-block:: console

   $ sudo snap install --dangerous --classic ./workshop_0.1.0_amd64.snap


Run
~~~

The snap installs two major components:

- The :program:`workshopd` daemon that exposes a REST API
- The :program:`workshop`
  :ref:`CLI tool <exp_workshop_cli>`
  that uses this API to command |project_markup|

The daemon starts automatically after installation;
the CLI tool is run manually:

.. code-block:: console

   $ workshop --help


Launch a workshop
-----------------

Having installed |project_markup|,
use it to define, launch, start and stop your first
:ref:`workshop <exp_workshop>`.


.. _tut_define:

Define
~~~~~~

#. Create a
   :ref:`project directory <exp_project>`
   named :file:`hello-workshop`:

   .. code-block:: console

      $ mkdir hello-workshop
      $ cd hello-workshop


#. In the project directory,
   create a
   :ref:`workshop definition <exp_workshop_def>`
   named :file:`.workshop.golang.yaml`:

   .. code-block:: yaml
      :caption: .workshop.golang.yaml

      name: golang
      base: ubuntu@22.04
      sdks:
        go:
          channel: latest/stable


#. Make sure |project_markup| can find the definition
   by *listing* the workshops
   in the project directory:

   .. code-block:: console

      $ workshop list

          Project                Workshop   Status  Notes
          ~/hello-workshop       golang     Off     -


   Note that a newly created workshop is *Off*.


.. _tut_launch:

Launch
~~~~~~

To prepare a workshop for action,
you :ref:`launch <ref_workshop_launch>` it:

.. code-block:: console

   $ workshop launch golang


Now, the workshop is *Ready*
to build, debug and run code.

To make sure |project_markup| watches the changes in the project directory,
move it, then run :command:`workshop list`:

.. code-block:: console

   $ cd ..
   $ mv hello-workshop hi-workshop
   $ cd hi-workshop
   $ workshop list

       Project                Workshop   Status  Notes
       ~/hi-workshop          golang     Ready   -

Mind that the workshop stays operational with no extra steps.

.. note::

   Check the recent :ref:`changes <ref_workshop_changes>`
   to see what goes into launching a workshop:

   .. code-block:: console

      $ workshop changes

         ID  Status  Spawn               Ready               Summary
         34  Done    today at 11:32 GMT  today at 11:33 GMT  Launch "golang" workshop

   To investigate this change in detail,
   supply its ID to the
   :ref:`tasks <ref_workshop_tasks>`
   command:

   .. code-block:: console

      $ workshop tasks 34

         ID   Status  Spawn               Ready               Summary
         133  Done    today at 11:32 GMT  today at 11:32 GMT  Create new "golang" workshop
         134  Done    today at 11:32 GMT  today at 11:32 GMT  Mount project directory "hello-workshop"
         135  Done    today at 11:32 GMT  today at 11:32 GMT  Start "golang" workshop
         136  Done    today at 11:32 GMT  today at 11:32 GMT  Retrieve "go" SDK from channel "latest/stable"
         137  Done    today at 11:32 GMT  today at 11:32 GMT  Install "go" SDK
         138  Done    today at 11:32 GMT  today at 11:33 GMT  Link "go" SDK
         139  Done    today at 11:33 GMT  today at 11:33 GMT  Run hook "setup-base" for "go" SDK
         140  Done    today at 11:33 GMT  today at 11:33 GMT  Auto-connect interfaces of "go" SDK

   Here, the project directory is mounted to the workshop as :file:`/project/`;
   the workshop is *started*, in other words, brought online;
   then the :samp:`go` SDK,
   which was referenced in the :ref:`definition <tut_define>`,
   is retrieved, installed and set up inside the workshop;
   finally, the SDK is connected to the host system
   via an :ref:`interface <exp_interfaces_plugs_slots>`.

   For details, see :ref:`exp_changes_tasks`.


Start and stop
~~~~~~~~~~~~~~

If you're done with the workshop for now,
*stop* it to conserve resources:

.. code-block:: console

   $ workshop stop golang


To resume, start the workshop again:

.. code-block:: console

   $ workshop start golang


Both commands operate gracefully,
waiting for the workshop to comply.


.. _tut_refresh:

Refresh a workshop
------------------

When an aspect of the workshop changes,
refresh it to pick up the update.


When base or SDKs update
~~~~~~~~~~~~~~~~~~~~~~~~

If the
:ref:`base <exp_workshop_base>`
or
:ref:`SDKs <exp_sdk>`
in the
:ref:`definition <exp_workshop_def>`
are updated by their publishers,
:ref:`refresh <ref_workshop_refresh>` the workshop to update it:

.. code-block:: console

   $ workshop refresh golang


The workshop is rebuilt from the base image,
then the SDKs are retrieved from respective channels.

To refresh multiple workshops at once, list them all:

.. code-block:: console

   $ workshop refresh golang ...

.. note::

   The operation is transactional: If an error occurs,
   **all** changes in **all** listed workshops are reverted.


When definition changes
~~~~~~~~~~~~~~~~~~~~~~~

To switch bases,
realign SDK layout or toggle channels,
update the definition
and refresh the workshop:

.. code-block:: yaml
   :caption: .workshop.golang.yaml
   :emphasize-lines: 2, 5

   name: golang
   base: ubuntu@20.04
   sdks:
     go:
       channel: latest/edge


.. code-block:: console

   $ workshop refresh golang


.. _tut_refresh_wait_on_error:

Wait on error
~~~~~~~~~~~~~

If a refresh fails, any changes are reverted by default;
to pause instead,
add the :option:`!--wait-on-error` option:

.. code-block:: console

   $ workshop refresh --wait-on-error golang

       ERROR command failed with an error code (1): The edge version is not stable

       cannot refresh; fix the errors reported by "workshop info",
       then run "workshop refresh --continue golang".
       To abort and revert, run "workshop refresh --abort golang".

All progress is saved, up to the specific *task* that caused the error.
Then, you can explore the paused workshop
and choose to abort or continue the refresh operation.

To investigate the issue, check the recent
:ref:`changes <ref_workshop_changes>`:

.. code-block:: console

   $ workshop changes

       ID  Status  Spawn                Ready                Summary
       ...
       81  Error   ...                  ...                  ...


Having found the problematic change, explore its
:ref:`tasks <ref_workshop_tasks>`:

.. code-block:: console

   $ workshop tasks 81

       ...
       1391  Undone  today at 12:17       today at 12:18       Link "go" SDK
       1392  Error   today at 12:17       today at 12:18       Run hook "setup-base" for "go" SDK
       ...


.. note::

   For details, see :ref:`exp_changes_tasks`.


To continue the refresh operation:

.. code-block:: console

   $ workshop refresh --continue golang


To abort the operation and recover the last operational state:

.. code-block:: console

   $ workshop refresh --abort golang


.. note::

   Even with no errors,
   check the recent changes
   to see what goes into refreshing a workshop:

   .. code-block:: console

      $ workshop changes

         ID  Status  Spawn               Ready               Summary
         35  Done    today at 12:15 GMT  today at 12:16 GMT  Launch "golang" workshop

   To investigate this change in detail,
   supply its ID to the
   :ref:`tasks <ref_workshop_tasks>`
   command:

   .. code-block:: console

      $ workshop tasks 35

          ID   Status  Spawn               Ready               Summary
          141  Done    today at 12:15 GMT  today at 12:15 GMT  Create SDK state storage
          142  Done    today at 12:15 GMT  today at 12:15 GMT  Run hook "save-state" for "go" SDK
          143  Done    today at 12:15 GMT  today at 12:15 GMT  Disconnect interfaces of "go" SDK
          144  Done    today at 12:15 GMT  today at 12:15 GMT  Stash previous "golang" workshop
          145  Done    today at 12:15 GMT  today at 12:16 GMT  Create new "golang" workshop
          146  Done    today at 12:15 GMT  today at 12:16 GMT  Mount project directory "hi-workshop"
          147  Done    today at 12:15 GMT  today at 12:16 GMT  Start "golang" workshop
          148  Done    today at 12:15 GMT  today at 12:15 GMT  Retrieve "go" SDK from channel "latest/stable"
          149  Done    today at 12:15 GMT  today at 12:16 GMT  Install "go" SDK
          150  Done    today at 12:15 GMT  today at 12:16 GMT  Link "go" SDK
          151  Done    today at 12:15 GMT  today at 12:16 GMT  Run hook "setup-base" for "go" SDK
          152  Done    today at 12:15 GMT  today at 12:16 GMT  Auto-connect interfaces of "go" SDK
          153  Done    today at 12:15 GMT  today at 12:16 GMT  Run hook "restore-state" for "go" SDK
          154  Done    today at 12:15 GMT  today at 12:16 GMT  Remove SDK state storage
          155  Done    today at 12:15 GMT  today at 12:16 GMT  Remove "golang" workshop from stash


   This largely resembles the events at :ref:`launch <tut_launch>`;
   however, :ref:`SDK states <exp_sdk_state>` are *stashed*
   and :ref:`interfaces <exp_interfaces_plugs_slots>` are disconnected
   before the refresh actually starts.
   After updating the workshop on success
   or rolling back to the stashed version on failure,
   |project_markup| recovers SDK states
   and reconnects the interfaces.


.. _tut_exec:

Execute commands
----------------

When the workshop is *Ready*,
execute arbitrary commands in it using :ref:`ref_workshop_exec`:

.. code-block:: go
   :caption: main.go

   package main

   import "fmt"

   func main() {
     fmt.Println("Hello, Workshop")
   }


.. code-block:: console

   $ workshop exec golang go build main.go


Variable injection
~~~~~~~~~~~~~~~~~~

To define environment variables and visibly separate the command's options:

.. code-block:: console

   $ workshop exec golang --env GO111MODULE=off -- go build -x


Interactive shell
~~~~~~~~~~~~~~~~~

You can run an interactive shell as well:

.. code-block:: console

   $ workshop exec golang -- sudo su workshop -l
   workshop@golang-6b79e889:~$ pwd

     /home/workshop

   workshop@golang-6b79e889:~$ uname -a

The command above launches the login shell for :samp:`workshop`,
the default non-privileged user in a workshop.


Changes in project
~~~~~~~~~~~~~~~~~~

Any changes you introduce under :file:`/project/` inside the workshop
are persisted in the project directory, and vice versa:

.. code-block:: console

   $ touch outside.txt
   $ workshop exec golang -- bash -c "ls -l"
   $ workshop exec golang -- touch inside.txt
   $ ls -l



.. _tut_remove:

Remove a workshop
-----------------

If you don't need a workshop anymore,
:ref:`remove <ref_workshop_remove>` it:

.. code-block:: console

   $ workshop remove golang


This doesn't affect the files in the project directory,
including the workshop definition,
and any other content that was stored outside the workshop,
e.g. via the :ref:`content interface <exp_content_interface>`.

.. attention::

   Don't delete the project directory without removing the workshop first.


That was the last step of the tutorial;
you are now familiar with the essential operations |project_markup| provides
and have had your first taste of what it can accomplish for you.
