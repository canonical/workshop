.. _tut_get_started:

.. meta::
   :description: Practical introduction to workshops, guiding users through
                 defining, launching, and refreshing workshops, and executing commands in workshops.

Get started with workshops
==========================

.. @tests in tests/docs-tutorial/part-1/task.yaml

This is the first section of the :ref:`four-part series <tut_index>`;
a practical introduction
that takes you on a tour
of the essential |ws_markup| activities.

.. @artefact workshop (container)

A *workshop* is a development environment running in a container,
mapping your project to its contained dependencies.
Here, you will practice all the major steps
in the lifecycle of a workshop,
from :ref:`defining <tut_define>`, :ref:`launching <tut_launch>`,
and :ref:`refreshing <tut_refresh>` it
to :ref:`executing commands <tut_exec>` and
:ref:`shelling <tut_shell>` into the workshop.
The steps you're about to perform
cover most of your daily needs with |ws_markup|.


.. _tut_install:

Install |ws_markup|
-------------------

Install |ws_markup|,
upgrading the prerequisites if needed,
then ensure it runs.

.. @artefact installation
.. @artefact workshopd
.. @artefact workshop (CLI)


Prerequisites
~~~~~~~~~~~~~

|ws_markup| is supported on Ubuntu
and other :program:`snap`-enabled Linux distributions;
it is also compatible with Windows Subsystem for Linux (WSL2),
where it uses Btrfs instead of ZFS for storage.

|ws_markup| relies on
`LXD 6.8+ <https://canonical.com/lxd>`_
for low-level operation
and uses its
`REST API <https://canonical.com/lxd/docs/latest/restapi_landing/>`_
to handle individual *workshops*.

To install it from scratch with :program:`snap`:

.. code-block:: console

   $ sudo snap install --channel=6/stable lxd


To refresh an existing :program:`snap` installation:

.. code-block:: console

   $ sudo snap refresh --channel=6/stable lxd


.. note::

   If you prefer another installation method,
   see the available installation options in
   `LXD documentation
   <https://canonical.com/lxd/docs/latest/installing/>`__;
   after installation,
   make sure the
   `LXD daemon
   <https://canonical.com/lxd/docs/latest/explanation/lxd_lxc/#lxd-daemon>`__
   is enabled and running.
   If in doubt, refer to LXD documentation
   and your distribution's manuals for guidance.


Installation
~~~~~~~~~~~~

Install the snap using the
`--classic <https://snapcraft.io/docs/install-modes/>`_ option:

.. code-block:: console

   $ sudo snap install --classic workshop


.. _tut_define_launch:

Launch a workshop
-----------------

Now you'll learn how to define, launch, start and stop a workshop.


.. _tut_define:

Define, add SDKs
~~~~~~~~~~~~~~~~

First, you need to define a workshop.
A definition is a YAML file that is stored in your project directory;
it lists the components of the workshop to be instantiated at launch.

.. @artefact sdkcraft (CLI)
.. @artefact SDK
.. @artefact SDK publisher
.. @artefact SDK Store

A definition can list many moving parts;
perhaps, the most important are SDKs,
which are basic, predefined building blocks
of your development environment.

You reference SDKs from your workshop definition
to specify what you want to include in your workshop.
At runtime, |ws_markup| pulls and installs them,
providing the dependencies and packages required for your work,
while keeping the SDKs themselves isolated and manageable.

For demonstration purposes, assume we want to work with AI models using the
`Ollama <https://ollama.com/>`__ platform.
To do this, let's use the :samp:`ollama` SDK,
which provides a local AI model server.

.. @artefact sdk find

Before adding an SDK to a workshop,
search the SDK Store to confirm it exists
and check its publisher and current version:

.. code-block:: console

   $ sdk find ollama

     NAME    VERSION  PUBLISHER     SUMMARY
     ollama  0.20.2   Canonical     Get up and running with large language models


The query also matches an SDK's title, summary, description, or publisher,
so a broader keyword like :command:`sdk find ai`
can surface AI-related SDKs on the Store.

.. @artefact sdk info

To see which channels and bases are available for a specific SDK,
inspect its details:

.. code-block:: console

   $ sdk info ollama

     name:       ollama
     publisher:  Canonical (canonical)
     license:    MIT
     website:    https://github.com/canonical/ollama-sdk

     Get up and running with Llama 3.3, ...

     CHANNELS
       CHANNEL        VERSION  BUILD       BASE          REV   SIZE
       latest/stable  0.20.2   2026-04-15  ubuntu@24.04    7  2.27GB
                                           ubuntu@22.04    8  2.27GB
       ...
       cpu/stable     0.20.2   2026-04-15  ubuntu@24.04    2  15.22MB
                                           ubuntu@22.04    5  15.22MB
       cpu/candidate  ^
       cpu/beta       ^
       cpu/edge       ^


The :samp:`CHANNELS` table lists each track
(here, :samp:`latest`, :samp:`cpu`, :samp:`cuda`, :samp:`rocm`, and :samp:`vulkan`)
at four risk levels (:samp:`stable`, :samp:`candidate`, :samp:`beta`, :samp:`edge`),
the bases each revision supports, and its on-disk size.
For this tutorial, we'll use :samp:`cpu/stable`,
which runs on any machine without GPU hardware.

.. @artefact project

For the project directory, create a new Python repository:

.. code-block:: console

   $ mkdir ollama-python-project
   $ cd ollama-python-project
   $ git init


Everything you handle with your workshop goes here:
your Python code, custom assets, and so on.

.. @artefact workshop definition

In the project directory,
scaffold a workshop definition with :command:`workshop init`,
passing the base, the SDKs, and their channels on the command line:

.. code-block:: console

   $ workshop init dev --sdks ollama/cpu/stable --base ubuntu@22.04

     "dev" workshop created at /home/user/ollama-python-project/.workshop/dev.yaml


Each :option:`!--sdks` entry can take the :samp:`<NAME>/<CHANNEL>` form,
so :samp:`ollama/cpu/stable` pins the :samp:`ollama` SDK
to its :samp:`cpu/stable` channel.
The command writes the definition to :file:`.workshop/dev.yaml`:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 4-5

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: ollama
       channel: cpu/stable


Here, the specific version to retrieve from the SDK Store
comes from the :samp:`cpu/stable` channel of the :samp:`cpu` track.

To confirm that |ws_markup| sees the definition,
list the workshops in the project directory:

.. @artefact workshop list

.. code-block:: console

   $ workshop list

     WORKSHOP  STATUS  NOTES
     dev       Off     -


.. note::

   |ws_markup| ships shell completion for Bash, Zsh, Fish, and PowerShell;
   if you installed via snap, it is already enabled.
   Press Tab as you type any :program:`workshop` command
   to autocomplete subcommands, flags, and arguments.

   Completion is context-aware:
   each command offers only values that make sense for it.
   For instance, :command:`workshop start` autocompletes
   from *Stopped* workshops only,
   :command:`workshop stop` from *Ready* ones,
   and :command:`workshop connect` autocompletes available plugs
   and then the matching slots.


As the command output suggests, your newly defined workshop is *Off*,
so it needs to be launched.

.. note::

   The command lists all workshops within the project;
   the tutorial focuses on a single-workshop setup,
   but your project can have multiple workshops defined.

   For a detailed explanation of the workshop status values,
   see the :ref:`exp_workshop_status` section.


.. note::

   The tutorial uses Ollama for demonstration purposes only.
   This doesn't imply that |ws_markup| is intended solely for AI;
   quite the contrary, it's envisioned as language-neutral and framework-agnostic.


.. _tut_launch:

Launch, start, and stop
~~~~~~~~~~~~~~~~~~~~~~~

To get a workshop ready for use, you launch it:

.. @artefact workshop launch

.. code-block:: console

   $ workshop launch

     "dev" launched


Once the workshop is launched,
you can start using it to build, debug, and run your code.

After launching, check the runtime information
to see what went into your workshop:

.. @artefact workshop info

.. code-block:: console

   $ workshop info

     name:     dev
     base:     ubuntu@22.04
     project:  /home/user/ollama-python-project
     status:   ready
     notes:    -
     sdks:
       system:
         installed:  (1)
       ollama:
         tracking:   cpu/stable
         installed:  0.20.2  2026-04-15  (5)
         mounts:
           models:
             host-source:      .../6b79e889/dev/mount/ollama/models
             workshop-target:  /home/workshop/.ollama/models


The output looks like the :ref:`definition <tut_define>`
with extra details such as the :ref:`mounts <tut_interfaces>`;
ignore these for now.

.. @artefact sdk list

While :command:`workshop info` shows the SDKs from *one* workshop's
perspective, :command:`sdk list` reports every SDK volume currently
stored on the machine, regardless of which workshop pulled it:

.. code-block:: console

   $ sdk list

   NAME    VERSION  REV  SIZE
   ollama  0.20.2     5  15.22MB
   system  -          1  25.09kB


Each row is a distinct SDK volume on disk.
Until you launch a workshop that references an SDK,
it won't appear here;
at launch, |ws_markup| has pulled the SDK from the SDK Store
and the revision shown matches the one in :command:`workshop info`.

.. @artefact workshop .lock

After launch, |ws_markup| tracks the project directory
using a hidden :file:`.workshop.lock` file
that must remain in the project directory
and **not be copied or stored externally**, e.g., in a repository.

You only need to launch a workshop once after defining it;
after any substantial changes to it,
you do a :ref:`refresh <tut_refresh>`.
Otherwise, the workshop is just a fancy container
that can be started and stopped.

The workshop starts automatically at launch,
but you can also stop and restart it at will.
Suppose you want to free up some resources, so you stop the workshop:

.. @artefact workshop stop

.. code-block:: console

   $ workshop stop

This changes the status of the workshop to *Stopped*.

To make it *Ready* again, start the workshop:

.. @artefact workshop start

.. code-block:: console

   $ workshop start


Both commands work gracefully,
waiting for the workshop to comply:

- :command:`workshop stop` doesn't destroy the workshop,
  unlike :ref:`remove <tut_remove>`

- :command:`workshop start` doesn't build it from scratch,
  unlike :ref:`launch <tut_launch>` or :ref:`refresh <tut_refresh>`


In the next step, you'll refresh an existing workshop.

.. note::

   If issues arise now or later, see these guides:
   :ref:`how_troubleshoot` and
   :ref:`how_debug_issues_workshops`.


.. note::

   Consider adding the :file:`.workshop.lock` file
   to your :file:`.gitignore` or similar ignore files:

   .. code-block:: console

      $ echo ".workshop.lock" >> .gitignore


   In contrast, the :file:`.workshop/` directory, which holds your definition,
   is *meant* to be stored in a repository;
   if your :file:`.gitignore` file uses rules
   such as "ignore everything except these files and directories,"
   add them to the list of explicitly tracked items.


.. _tut_refresh:

Refresh a workshop
------------------

Sometimes the base or the SDKs
listed in your :ref:`workshop definition <tut_define>`
are updated by their publishers.
Alternatively,
you may have changed the definition to switch bases,
add and remove SDKs, or toggle their channels.
A good example is when a new Ubuntu LTS version is released and,
as a result,
a new base image becomes available.
In either case,
you must refresh the workshop to apply the updates.

For example, change the base and the SDK channel in your definition
and refresh the workshop:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 2,5

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: vulkan/stable

.. @artefact workshop refresh

.. code-block:: console

   $ workshop refresh


After the refresh, :command:`sdk list` may show multiple revisions
of :samp:`ollama` if the previous :samp:`cpu/stable` volume is still
on disk alongside the freshly pulled :samp:`vulkan/stable` one.

Running :command:`workshop refresh` is similar to a :ref:`launch <tut_launch>`.
However, it ensures the workshop remains operational.
If issues occur, a refresh rolls back to a previous stable condition,
whereas a failed launch has no condition to revert to and just fails.

SDKs in a workshop diagnose themselves during launch and refresh;
if an SDK fails to set up,
the entire change is rolled back to keep the workshop operational.

Finally,
to discard **any changes made inside the workshop**
since the last successful launch or refresh,
run :command:`workshop restore`.
Unlike the automatic rollback on a failed refresh,
this is a deliberate action
that reverts the workshop to the most recent snapshot
and resets it to its default state.

Now that you can launch, refresh, start and stop a workshop,
let's move on to more practical purposes.


.. _tut_exec:

Execute commands
----------------

When the workshop is *Ready*,
you can run arbitrary commands in it.
In this tutorial, we're working with Ollama AI models,
and we've already created a Python project directory
to serve as our workspace.

First, let's put our example workshop to practical use;
download a simple AI model *inside the workshop*
using the :command:`workshop exec` command.
We'll use the :samp:`tinyllama` model, which is small and quick to download:

.. @artefact workshop exec

.. code-block:: console

   $ workshop exec dev -- ollama run tinyllama


This downloads and then runs the :samp:`tinyllama` model.
The model will be stored in the mounted :file:`models/` directory,
so it persists between workshop refreshes.
Quit the Ollama console by pressing :samp:`Ctrl+D`.

You can also list the available models:

.. code-block:: console

   $ workshop exec dev -- ollama list

     NAME                ID              SIZE      MODIFIED
     tinyllama:latest    2644915ede35    637 MB    24 seconds ago


Furthermore, your work files and deliverables,
however complex they may be, can reside on the host system,
while the toolchain is transparently confined and managed by |ws_markup|.
This enables you to focus on your project,
switching when needed between language and framework versions or base images.

Next, we'll explore the remaining aspects of your daily workshop usage.

.. note::

   |ws_markup| also integrates with modern IDEs.
   For instance, see these guides:
   :ref:`how_vscode_connect_remote`.


.. _tut_shell:

Interactive shell
~~~~~~~~~~~~~~~~~

Besides running individual commands,
you can open an interactive shell
if you need to perform multiple operations within a session.
|ws_markup| runs the login shell
for the default nonprivileged user,
who's also named :samp:`workshop`:

.. @artefact workshop shell

.. code-block:: console

   $ workshop shell
   workshop@dev:/project$ pwd

     /project

   workshop@dev:/project$ lsb_release -a

     ...
     Distributor ID: Ubuntu
     Description:    Ubuntu 24.04.3 LTS
     Release:        24.04
     Codename:       noble

   workshop@dev:/project$ exit


.. _tut_actions:

Reusable actions
~~~~~~~~~~~~~~~~

.. @artefact workshop actions

For complex commands that you run often,
define actions in your workshop definition
to invoke them with :command:`workshop run`.

Add an :samp:`actions` section to :file:`.workshop/dev.yaml`:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 6-7

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: vulkan/stable
   actions:
     pull: ollama pull "$@"


The :samp:`"$@"` expansion forwards every argument
supplied after the action name
into the action's :program:`bash` script.

Pull a model through the new action;
:samp:`"$@"` forwards :samp:`tinyllama` into :command:`ollama pull`:

.. code-block:: console

   $ workshop run dev -- pull tinyllama


Finally, actions are parsed on every :command:`workshop run`,
so there's no need to refresh the workshop after editing them.

.. note::

   See :ref:`how_add_actions` for more on writing and running actions.


.. _tut_project_updates:

Project directory updates
~~~~~~~~~~~~~~~~~~~~~~~~~

Remember that the project directory is mounted as :file:`/project/`
when the workshop is launched;
any changes to :file:`/project/` from inside the workshop
are visible in the project directory, and vice versa:

.. code-block:: console

   $ touch created_outside.txt
   $ workshop exec dev -- ls /project/

     ...  created_outside.txt  ...

   $ workshop exec dev -- touch /project/created_inside.txt
   $ ls

     ...  created_inside.txt  created_outside.txt  ...


Next, let's dive into how changes and tasks work
to track your workshop activities.


.. _tut_changes_tasks:

Track changes and tasks
-----------------------

To see how |ws_markup| keeps track of its activities around a project,
check out the recent major operations, or changes,
with :command:`workshop changes`:

.. @artefact workshop changes

.. code-block:: console

   $ workshop changes

     ID  STATUS  SPAWN               READY               SUMMARY
     1   Done    today at 09:26 CET  today at 09:27 CET  Launch "dev" workshop
     ...
     4   Done    today at 09:32 CET  today at 09:34 CET  Refresh "dev" workshop


Changes are enacted atomically to ensure workshops stay operational.
Any change must have all its smaller steps, or tasks, succeed;
otherwise, it will be reverted.

To look at the latest change,
run the :command:`workshop tasks` command without an argument.
To find out which tasks went into a certain change,
pass the change ID to the command:

.. @artefact workshop tasks

.. code-block:: console

   $ workshop tasks 4

     STATUS   DURATION  SUMMARY
     Done    2m17.389s  Download "ubuntu@24.04" base image
     Done        113ms  Retrieve "system" SDK
     Done    2m59.777s  Retrieve "ollama" SDK from channel "vulkan/stable"
     Done        443ms  Create SDK state storage
     Done        581ms  Run hook "save-state" for "system" SDK
     Done        449ms  Run hook "save-state" for "ollama" SDK
     Done         54ms  Disconnect interfaces of "ollama" SDK
     ...
     Done        528ms  Setup "system" SDK profile



This lists all the tasks and includes logs for some of them;
each task expresses a simple token of logic,
such as running a hook or connecting an interface.


Next steps
----------

This was the last step in this tutorial section;
you are now familiar with the essential operations provided by |ws_markup|
and have had your first taste of what it can do for you.

Your next step is to learn how to work with interfaces;
proceed to the :ref:`tut_work_with_interfaces` section.
