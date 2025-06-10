:slug: tutorial
.. _tutorial:

Tutorial
========

This is a practical introduction
that takes you on a tour
of the essential |ws_markup| activities.

.. @artefact workshop (container)

A workshop is an environment
that maps your project to its contained dependencies.
Here, you will practise all the major steps
in the life cycle of a *workshop*,
from :ref:`defining <tut_define>`, :ref:`launching <tut_launch>`
and :ref:`refreshing <tut_refresh>` it
to :ref:`executing commands <tut_exec>`,
:ref:`shelling <tut_shell>` into the workshop
and finally :ref:`removing <tut_remove>` it.
The actions you're about to perform
cover most of your daily needs with |ws_markup|.


.. _tut_install:

Install |ws_markup|
-------------------

Check the prerequisites,
build and install |ws_markup|,
then ensure it runs.


Prepare LXD
~~~~~~~~~~~

|ws_markup| relies on
`LXD 6.3+ <https://canonical.com/lxd>`_
for low-level operation
and uses its
`API <https://documentation.ubuntu.com/lxd/latest/restapi_landing/>`_
to handle individual *workshops*.
Check whether it's properly configured:

.. code-block:: console

   $ lxc info | grep 'server_version:'

     server_version: "6.3"


If the command displays an older version
or returns an error indicating LXD is missing,
install a recent LXD version with :program:`snap`.
To install it from scratch:

.. code-block:: console

   $ sudo snap install lxd --channel=6/stable


To refresh an existing installation:

.. code-block:: console

   $ sudo snap refresh lxd --channel=6/stable


.. note::

   For other ways to install LXD,
   see the available installation options in
   `LXD documentation
   <https://documentation.ubuntu.com/lxd/latest/installing/>`_.
   Also, you need to ensure the
   `LXD daemon
   <https://documentation.ubuntu.com/lxd/latest/explanation/lxd_lxc/#lxd-daemon>`_
   is enabled and running.
   Again, refer to LXD documentation
   and your distribution's manuals for guidance.


With LXD properly installed, initialised and started,
proceed to installing |ws_markup|.


Install
~~~~~~~

.. @artefact installation
.. @artefact workshopd
.. @artefact workshop (CLI)

Download the latest snap from |ws_markup|'s "Releases" page on GitHub:
:literalref:`https://github.com/canonical/workshop/releases/`


Browse to the download directory and install the snap using the options
`--dangerous <https://snapcraft.io/docs/install-modes>`_
and
`--classic <https://snapcraft.io/docs/install-modes>`_:

.. code-block:: console

   $ sudo snap install --dangerous --classic ./workshop_0.1.17_amd64.snap


Shell integration (optional)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

|ws_markup| features shell completion for popular shells
such as :program:`bash`, :program:`zsh` and :program:`fish`.

Bash completion is configured automatically;
manual setup instructions are available for all shells:

.. code-block:: console

   $ workshop completion bash -h
   $ workshop completion fish -h
   $ workshop completion zsh -h


With completion enabled, you can press the :kbd:`Tab` key while typing a command
to quickly substitute suitable subcommands, flags and arguments.


.. _tut_define_launch:

Launch a workshop
-----------------

Now you'll learn how to define, launch, start and stop a workshop.


.. _tut_define:

Define
~~~~~~

First, you need to define a workshop.
A definition lists the components of a workshop
to be instantiated at launch
and is stored in your project directory.

.. @artefact sdkcraft (CLI)
.. @artefact SDK
.. @artefact SDK publisher
.. @artefact SDK Store

A definition can list many moving parts;
in this tutorial, we'll be focusing on SDKs,
which are the basic units of a workshop's functionality.
At run-time, |ws_markup| pulls and installs them,
providing the dependencies and packages required for your work,
while keeping the SDKs themselves isolated and manageable.

For demonstration purposes, assume we want to build some Go code.
To do this, let's use the sample :samp:`go` SDK,
which was already defined, built and published in the SDK Store
by the |ws_markup| team.

.. tip::

   The tutorial uses Go samples for demonstration purposes only.
   This doesn't imply that |ws_markup| is intended solely for Go;
   quite the contrary, it's envisioned as language-neutral and framework-agnostic.


.. @artefact project

Create a project directory named :file:`hello-workshop`:

.. code-block:: console

   $ mkdir ./hello-workshop
   $ cd ./hello-workshop


Everything you handle with your workshop goes here:
your source code, custom assets and so on.

.. @artefact workshop definition

In the project directory,
create a workshop definition named :file:`workshop.yaml`:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 4

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: jammy/stable


Here, the SDK is referenced as :samp:`go`,
and the specific version to retrieve from the SDK Store
comes from the :samp:`jammy/stable` channel.

To confirm that |ws_markup| sees the definition,
list the workshops in the project directory:

.. @artefact workshop list

.. code-block:: console

   $ workshop list

     Project                Workshop   Status  Notes
     ~/hello-workshop       dev        Off     -


As the output suggests, your newly defined workshop is *Off*,
so it needs to be launched.


.. _tut_launch:

Launch, start and stop
~~~~~~~~~~~~~~~~~~~~~~

To get a workshop ready for use, you launch it:

.. @artefact workshop launch

.. code-block:: console

   $ workshop launch


Now, the workshop is *Ready*;
you can start using it to build, debug and run your code.

.. note::

   If issues arise now or later, see these guides:
   :ref:`how_troubleshoot` and
   :ref:`how_debug_issues_workshops`.


After launching, check the run-time information
to see what went into your workshop:

.. @artefact workshop info

.. code-block:: console

   $ workshop info

     name:     dev
     base:     ubuntu@22.04
     project:  /home/user/hello-workshop
     status:   ready
     notes:    -
     sdks:
       go:
         tracking:   jammy/stable
         installed:  1.23.0  2024-08-15  (51)
         mounts:
           mod-cache:
             host-source:      .../6b79e889/dev/mount/go/mod-cache
             workshop-target:  /home/workshop/go/pkg/mod


The output looks like the :ref:`definition <tut_define>`
with extra details such as the :ref:`mounts <tut_interfaces>`;
ignore these for now.

.. @artefact workshop .lock

After launch, |ws_markup| starts tracking the project directory.
The workshop stays operational with no extra steps on your part
by using a hidden :file:`.lock` file that must remain in the project directory
and not be copied or stored externally, e.g. in a repository.

To see how |ws_markup| keeps track of the directory,
check out the recent major operations, or changes:

.. @artefact workshop changes

.. code-block:: console

   $ workshop changes

     ID  Status  Spawn               Ready               Summary
     34  Done    today at 11:32 GMT  today at 11:33 GMT  Launch "dev" workshop


To find out which smaller steps, or tasks, went into a certain change,
pass the change ID to the :command:`workshop tasks` command.
To look at the latest change,
use :command:`workshop tasks` without the argument:

.. @artefact workshop tasks

.. code-block:: console

   $ workshop tasks

     ID   Status  Spawn               Ready               Summary
     132  Done    today at 11:32 GMT  today at 11:32 GMT  Retrieve "system" SDK
     133  Done    today at 11:32 GMT  today at 11:32 GMT  Retrieve "go" SDK from channel "latest/stable"
     ...
     144  Done    today at 11:32 GMT  today at 11:33 GMT  Run hook "setup-base" for "go" SDK
     ...

     ......................................................................
     Run hook "setup-base" for "go" SDK

     2024-08-25T11:34:53+00:00 INFO go 1.23.0 from Canonical** installed


This lists all the tasks and includes logs for some of them.

You only need to launch a workshop once after defining it;
for any subsequent changes, you can do a :ref:`refresh <tut_refresh>`.
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


.. _tut_refresh:

Refresh a workshop
------------------

Sometimes the base or the SDKs
listed in your :ref:`workshop definition <tut_define>`
are updated by their publishers.
Alternatively,
you may have changed the definition to switch bases,
add and remove SDKs or toggle their channels.
In either case,
you must refresh the workshop to apply the updates.

To do so, change the base and the SDK channel in your definition
and refresh the workshop:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 2,5

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: go
       channel: noble/stable

.. @artefact workshop refresh

.. code-block:: console

   $ workshop refresh


Running :command:`workshop refresh` is similar to a :ref:`launch <tut_launch>`.
However, it ensures the workshop remains operational.
If issues occur, a refresh rolls back to a previous stable condition,
whereas a failed launch has no condition to revert to and just fails.

Now that you can launch, refresh, start and stop a workshop,
let's move on to more practical purposes.


.. _tut_exec:

Execute commands
----------------

When the workshop is *Ready*,
you can run arbitrary commands in it.
In this tutorial, we're building Go code, so let's write some.

In the project directory, save this code as :file:`main.go`:

.. code-block:: go
   :caption: main.go

   package main

   import "fmt"

   func main() {
     fmt.Println("Hello, Workshop")
   }


.. note::

   |ws_markup| also integrates with modern IDEs.
   For instance, see this guide:
   :ref:`how_vscode_workshops`.


Next, build it *inside the workshop* using the :command:`workshop exec` command:

.. @artefact workshop exec

.. code-block:: console

   $ workshop exec dev -- go build main.go


This runs the Go version installed by the :samp:`go` SDK.
The resulting binary, built within the workshop environment,
is now available in the project directory.

**This is the single most important part of the tutorial**;
your deliverables, however complex they are, end up on the host system,
while the tool chain is transparently confined and managed by |ws_markup|.

Next, we'll explore the remaining aspects of your daily workshop usage.


.. _tut_shell:

Interactive shell
~~~~~~~~~~~~~~~~~

Besides running individual commands,
you can open an interactive shell
if you need to perform multiple operations within a session.
|ws_markup| runs the login shell
for the default non-privileged user,
who's also named :samp:`workshop`:

.. @artefact workshop shell

.. code-block:: console

   $ workshop shell
   workshop@dev-6b79e889:~$ pwd

     /home/workshop

   workshop@dev-6b79e889:~$ uname -a
   workshop@dev-6b79e889:~$ exit


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
   $ workshop exec dev -- touch /project/created_inside.txt
   $ ls


This isn't the only way the host interacts with the workshop;
let's dive into how interfaces operate.


.. _tut_interfaces:

Work with interfaces
--------------------

.. @artefact interface
.. @artefact system SDK

SDKs use interfaces to interact in an organised manner,
exposing the resources they provide via slots and consuming them via plugs;
the layout of these plugs and slots is defined by the SDK publishers.

For uniformity, security and control,
various host system capabilities (camera, GPU, and so on)
are also exposed to the workshop via the same interface mechanism
with a designated system SDK.

To check out the connected interfaces of a workshop, list the connections:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections

     Interface  Plug              Slot              Notes
     mount      dev/go:mod-cache  dev/system:mount  -


This lists a mount interface plug named :samp:`dev/go:mod-cache`.
As seen in the :command:`workshop info` output,
it was automatically connected at :ref:`launch <tut_launch>`
to the :samp:`dev/system:mount` slot,
indicated by the ellipsis in the :samp:`host-source` path.

Some interfaces are auto-connected, while some are not;
this usually depends on their purpose.

In any case, you can connect and disconnect interfaces at will:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop disconnect dev/go:mod-cache
   $ workshop connect dev/go:mod-cache :mount


You can remount a mount interface plug to a new location on the host:

.. @artefact workshop remount

.. code-block:: console
   :emphasize-lines: 14

   $ workshop remount dev/go:mod-cache ~/mod/
   $ workshop info

     name:     dev
     base:     ubuntu@24.04
     project:  /home/user/hello-workshop
     status:   ready
     notes:    -
     sdks:
       go:
         tracking:   noble/stable
         installed:  1.23.3  2024-11-09  (54)
         mounts:
           mod-cache:
             host-source:      /home/user/mod
             workshop-target:  /home/workshop/go/pkg/mod


This makes :file:`/home/user/mod/` on the host
act as the Go modules cache for the workshop.

Lastly, you can add plugs and slots to the SDKs in the workshop definition,
allowing you to tailor the initial plug and slot layout to your requirements.
For instance, you could use the tunnel interface
with the system SDK to connect to a server running in the workshop.

.. @artefact tunnel interface

For a quick demo, let's install `Caddy <https://caddyserver.com/>`_
to serve files over HTTP:

.. code-block:: console

   $ workshop exec --env GOBIN=/project dev -- go install github.com/caddyserver/caddy/v2/cmd/caddy@latest
   $ cat <<EOF > Caddyfile
   :8080 {
           file_server
   }
   EOF
   $ echo 'Hello, Workshop!' > index.html


This builds Caddy inside the workshop,
installs it to the project directory,
configures it to run as a file server at port 8080
and creates an index file.

.. note::

   We added the index file to the project directory on the host;
   however, the server will be able to access it
   because the project directory is mounted inside the workshop.


To configure the tunnel interface,
add the following lines to the definition:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 6-14

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: go
       channel: noble/stable
       slots:
         caddy:
           interface: tunnel
           endpoint: localhost:8080
     - name: system
       plugs:
         caddy:
           interface: tunnel
           endpoint: localhost:8080


First, this defines a :samp:`go:caddy` slot under the :samp:`go` SDK,
used to expose the server running inside the workshop.
This slot isn't part of the SDK by default;
it's defined for this workshop only,
so other instances of the :samp:`go` SDK in other workshops won't have it.

Additionally, this adds a plug named :samp:`system:caddy`
to indicate that the system SDK in this workshop
can connect to a tunnel interface slot and expose it in the host system.

Refresh the workshop to enable the tunnel;
|ws_markup| matches the plug to the slot using their names,
then validates and enables the connection.
Check the result using :command:`workshop info`:

.. code-block:: console

   $ workshop refresh
   $ workshop info

     ...
     sdks:
       system:
         tunnels:
           server:
             from:  127.0.0.1:8080/tcp
             to:    127.0.0.1:8080/tcp
     ...

Then start the server at port 8080 (the slot):

.. code-block:: console

   $ workshop exec dev -- ./caddy start


By default,
:command:`exec` uses the :file:`/project/` directory in the workshop
as the current working directory
so Caddy will serve the files in it.
Finally, test the server on the host at port 8080 (the plug):

.. code-block:: console

   $ curl localhost:8080

     Hello, Workshop!


.. note::

   For additional details of using the tunnel interface, see this guide:
   :ref:`how_forward_ports`.


.. _tut_remove:

Remove a workshop
-----------------

We're at the end of our tutorial;
the only thing left is the cleanup.

If you no longer need your workshop,
remove it:

.. @artefact workshop remove

.. code-block:: console

   $ workshop remove


This doesn't affect the files in the project directory,
including the workshop definition,
or any other content that was stored outside the workshop
(e.g. using the :ref:`mount interface <tut_interfaces>`
with a custom :command:`workshop remount` location;
however, the content in *default* mount locations will be deleted).

.. important::

   Don't delete the project directory without first removing the workshop.
   Otherwise, you'll need to manually delete the orphaned workshops;
   for help, see this guide: :ref:`how_troubleshoot_lxc`.


Next steps
----------

This was the last step in the tutorial;
you are now familiar with the essential operations provided by |ws_markup|
and have had your first taste of what it can do for you.

|ws_markup| is built around the concept of SDKs;
you next step is to look at the different ways of building them.

- First, you can graft a special *sketch SDK* onto the workshop
  with the :command:`workshop sketch-sdk` command
  to run a quick local experiment while designing an SDK.
  For details, see this guide: :ref:`how_sketch`.


- If you wish to build and publish a full-fledged SDK to the Store
  instead of going for a local-only option,
  proceed to this guide: :ref:`how_sdkcraft`.
  The :ref:`ROS 2 how-to guides <how_ros2>`
  provide a real-world example
  to detail the process of building an SDK
  and using it in |ws_markup|.


Finally, if you need a more descriptive overview of |ws_markup|,
refer to the :ref:`explanation <exp_index>` section.
For comprehensive details,
explore the :ref:`reference <ref_index>` section.
If you're looking for advanced practical steps,
see the :ref:`how-to guides <how_index>`.
