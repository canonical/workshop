.. _how_vscode_connect_remote:

.. meta::
   :description: How-to guide on developing inside a workshop from VS Code
                 with the Workshop extension: create a workshop, reopen the
                 project in it, refresh it after definition changes, and
                 switch between the host and the workshop.

How to develop in a workshop with VS Code
=========================================

.. @tests in tests/docs-how-to/connect-vscode/task.yaml

.. @artefact workshop (container)

The `Workshop extension for VS Code <https://marketplace.visualstudio.com/items?itemName=canonical.workshop>`_
opens your project inside a workshop
while VS Code itself keeps running on the host.
It lists the project's workshops in a side bar,
creates new ones,
connects the window to a workshop over SSH,
and offers to refresh a workshop when its definition changes.

The steps below take a Go project
from an empty directory to a server running inside a workshop,
then change, break, and repair the workshop's definition
and switch between the host and the workshop.

.. figure:: /images/vscode/connected-window.png
   :align: center
   :alt: VS Code window connected to the dev workshop,
         with the Workshops side bar expanded
         and the remote indicator reading SSH: dev.hello-workshop.wp

   VS Code running inside the :samp:`dev` workshop


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- VS Code 1.90 or later installed on the host.

The UI labels and screenshots match version 0.5.2
of the Workshop extension;
other versions may differ in details.


Install the extension
---------------------

Install the extension from the Visual Studio Marketplace:

#. In VS Code, open the Extensions view
   by pressing :kbd:`Ctrl+Shift+X`.

#. Search for :samp:`Workshop`
   and pick the extension published by Canonical.

#. Click :guilabel:`Install`.

Alternatively, install it from a terminal:

.. code-block:: console

   $ code --install-extension canonical.workshop


The extension depends on `Remote - SSH <https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh>`_,
which VS Code installs alongside it.
Once installed, a :guilabel:`Workshop` icon appears in the Activity Bar,
opening the Workshops view.

.. note::

   The extension talks to the :program:`workshopd` daemon on the host.
   If the Workshops view says
   that Workshop doesn't appear to be installed or running,
   see :ref:`how_vscode_troubleshoot`.


Create a project and a workshop
-------------------------------

Start with a small Go program in a fresh directory;
Go itself isn't needed on the host,
as the workshop provides it.
Create :file:`hello-workshop/` with two files:

.. code-block:: go
   :caption: hello-workshop/hello.go

   package main

   import (
   	"fmt"
   	"net/http"
   	"runtime"
   )

   func main() {
   	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
   		fmt.Fprintf(w, "Hello from %s\n", runtime.Version())
   	})
   	fmt.Println("Listening on http://localhost:8080")
   	if err := http.ListenAndServe(":8080", nil); err != nil {
   		fmt.Println(err)
   	}
   }


.. code-block:: text
   :caption: hello-workshop/go.mod

   module hello

   go 1.27


Open the directory in VS Code,
for example with :command:`code hello-workshop` from a terminal,
then click the :guilabel:`Workshop` icon in the Activity Bar.
The project has no workshops yet,
so the Workshops view offers to create one:

.. figure:: /images/vscode/workshops-view-empty.png
   :align: center
   :alt: The Workshops view showing "No workshops found for this project"
         and an Add New Workshop button

   The Workshops view for a project without workshops

.. @artefact workshop init

Click :guilabel:`Add New Workshop`
and follow the wizard:

#. In :guilabel:`Select SDKs`, tick :samp:`go`
   and press :kbd:`Enter`.
   The list shows the
   `reference SDKs <https://github.com/canonical/reference-sdks>`_
   published by Canonical,
   grouped by category;
   the info button next to an SDK opens its repository.

   .. figure:: /images/vscode/wizard-select-sdks.png
      :align: center
      :alt: The Select SDKs step of the wizard with the go SDK ticked

      Selecting the :samp:`go` SDK

#. In :guilabel:`Select a base`, keep :samp:`ubuntu@24.04`,
   marked as the default,
   and press :kbd:`Enter`.

#. In :guilabel:`Enter a name`, keep :samp:`dev`
   and press :kbd:`Enter`.

The wizard runs :command:`workshop init` for you,
opens the resulting definition in the editor,
and offers to reopen the window in the new workshop:

.. figure:: /images/vscode/created-workshop.png
   :align: center
   :alt: VS Code showing the generated .workshop/dev.yaml definition
         and a notification reading Created "dev". Reopen this window in the workshop?

   The definition created by the wizard

The definition lives in :file:`.workshop/dev.yaml`
and pins the SDK to the channel the wizard recommends:

.. code-block:: yaml
   :caption: .workshop/dev.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: go
       channel: 1.27/stable


Keep the notification open;
the next step uses it.

.. note::

   The wizard lists only the reference SDKs published by Canonical.
   To use any other SDK,
   add it to the definition by hand,
   as shown in :ref:`how_vscode_refresh`.


Reopen the project in the workshop
----------------------------------

Click :guilabel:`Reopen in Workshop` in the notification.
If you dismissed it,
hover over the :samp:`dev` row in the Workshops view
and click the :guilabel:`Reopen in Workshop` button that appears:

.. figure:: /images/vscode/workshops-view-off.png
   :align: center
   :alt: The Workshops view with the dev workshop in the Off state
         and its inline Reopen in Workshop and Open Definition File buttons

   The :samp:`dev` workshop before its first launch

The workshop is only defined at this point,
so the extension launches it first:
it creates the container from the base image,
installs the SDK,
and reports the progress in a notification.

.. figure:: /images/vscode/launch-progress.png
   :align: center
   :alt: A progress notification for the dev workshop
         showing the task in progress and its percentage

   Launch progress

When the workshop is ready,
VS Code reconnects the same window to it over SSH.
The window is inside the workshop when:

- the remote indicator in the bottom-left corner reads
  :samp:`SSH: dev.hello-workshop.wp`,
  the workshop's hostname;

- the Explorer shows the project under :file:`/project`,
  where the workshop mounts your project directory;

- the :samp:`dev` row in the Workshops view turns green.

The next time you reopen the project in the workshop,
the extension skips the launch:
it starts a stopped workshop or connects to a running one directly.

.. note::

   |ws_markup| sets up SSH access to workshops on its own.
   It configures the OpenSSH client on your local machine
   and maintains a certificate authority
   that signs an SSH *host* certificate for every workshop's SSH server
   and a *user* certificate authenticating you as the :samp:`workshop` user.
   Connecting to a workshop by its hostname
   needs no key management, passwords, or host-key prompts.


Run the code in the workshop
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Open the integrated terminal with :kbd:`Ctrl+\``;
it runs inside the workshop.
Confirm where you are and that Go is available:

.. code-block:: console

   workshop@dev:/project$ hostname

     dev
   workshop@dev:/project$ go version

     go version go1.27.1 linux/amd64


.. figure:: /images/vscode/terminal-go-version.png
   :align: center
   :alt: The integrated terminal inside the workshop
         showing the hostname and go version commands and their output

   The integrated terminal runs in the workshop

Then run the program:

.. code-block:: console

   workshop@dev:/project$ go run hello.go

     Listening on http://localhost:8080


The server listens inside the workshop,
but Remote - SSH forwards the port to the host automatically
and offers to open it in your browser.
Visit http://localhost:8080 on the host
to see the greeting from the workshop's Go runtime.
Press :kbd:`Ctrl+C` in the terminal to stop the server.

.. figure:: /images/vscode/port-forwarded.png
   :align: center
   :alt: A notification saying that the application running on port 8080
         is available, with an Open in Browser button

   Remote - SSH forwarding port 8080


Explore the Workshops view
--------------------------

Expand the :samp:`dev` row to see what the workshop is made of:
its base, its hostname,
and its SDKs with the channel and version of each.

.. figure:: /images/vscode/workshops-view-expanded.png
   :align: center
   :alt: The Workshops view with the dev row expanded,
         listing Base, Hostname, and SDKs with the go SDK

   Details of a running workshop

Hovering over the row reveals its actions:

- :guilabel:`Open Definition File` opens :file:`.workshop/dev.yaml`.

- :guilabel:`Refresh and Reopen` applies definition changes;
  see :ref:`how_vscode_refresh`.

- :guilabel:`Turn Off…`, in the row's context menu,
  removes the workshop container after a confirmation;
  see :ref:`how_vscode_switch`.

If the view ever looks out of date,
run :guilabel:`Workshop: Refresh Workshops` from the Command Palette
to re-read the workshop list from the daemon.


.. _how_vscode_refresh:

Change the definition and refresh the workshop
----------------------------------------------

Workshops are built from their definitions,
so adding a tool means editing the definition
and refreshing the workshop.
Hover over the :samp:`dev` row,
click :guilabel:`Open Definition File`,
and add the :samp:`uv` SDK for Python tooling:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 6

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: go
       channel: 1.27/stable
     - name: uv


As soon as you save the file,
the extension notices the change
and offers to apply it:

.. figure:: /images/vscode/definition-changed-prompt.png
   :align: center
   :alt: A notification reading "dev" definition changed. Refresh and reopen?
         with a Refresh and Reopen button

   The definition change prompt

Click :guilabel:`Refresh and Reopen`.
A workshop can't refresh itself from the inside,
so the window first returns to the project on the host,
then refreshes the workshop
and reopens it when the refresh completes:

.. figure:: /images/vscode/refresh-progress.png
   :align: center
   :alt: A progress notification for the dev workshop during a refresh

   Refresh progress

Back inside the workshop,
confirm the new SDK in the integrated terminal:

.. code-block:: console

   workshop@dev:/project$ uv --version

The reported version follows the SDK's current default channel
because the definition doesn't pin one.


.. note::

   The extension only offers to refresh workshops
   that have been launched before.
   For a workshop that's still only a definition,
   :guilabel:`Reopen in Workshop` launches it
   with the current definition instead.


Recover from a broken definition
--------------------------------

A definition can fail to apply,
for example because of a misspelled SDK name.
To see how the extension handles that,
open :file:`.workshop/dev.yaml` again
and change the SDK name to :samp:`goo`,
then click :guilabel:`Refresh and Reopen` in the prompt.

The window returns to the host as before,
but this time the daemon rejects the definition
before making any change to the workshop.
The extension opens the definition
next to a read-only :samp:`dev (error)` log
that carries the daemon's reason,
so you can fix the definition without leaving the editor:

.. figure:: /images/vscode/refresh-error.png
   :align: center
   :alt: VS Code showing .workshop/dev.yaml on the left
         and the dev (error) log on the right,
         reporting that the SDK is unknown

   A rejected definition and the reason for the rejection

Restore the SDK name, save,
and accept the prompt again.

An error can also occur halfway through a refresh,
for example when an SDK's setup hook fails.
In that case the refresh *pauses* instead of failing,
leaving the workshop in the :samp:`Waiting` state,
and the extension asks how to proceed:

.. figure:: /images/vscode/refresh-paused.png
   :align: center
   :alt: VS Code showing the definition and the error log side by side
         and a notification reading "dev" refresh is paused due to a failure,
         with Reopen and Debug and Abort buttons

   A paused refresh

- :guilabel:`Reopen and Debug` reopens the window in the paused workshop,
  where you can investigate with the integrated terminal.

- :guilabel:`Abort` reverts the refresh
  and brings the workshop back to its previous state.

While the refresh is paused,
the :samp:`dev` row offers :guilabel:`Continue Refresh`
to retry from the failed task after you've fixed the cause,
and :guilabel:`Abort Refresh` to revert.
The extension refreshes in the :option:`!--wait-on-error` mode of
:command:`workshop refresh`.


.. _how_vscode_switch:

Switch between the host and the workshop
----------------------------------------

To use tools installed only on the host,
or to edit the definition while the workshop is being rebuilt,
return to the host:
click the remote indicator in the bottom-left corner
and choose :guilabel:`Reopen Locally`:

.. figure:: /images/vscode/reopen-locally-menu.png
   :align: center
   :alt: The remote indicator menu with the Reopen Locally entry highlighted

   Reopen Locally in the remote indicator menu

The same command is available
as the :guilabel:`Reopen Locally` button in the Workshops view title
and as :guilabel:`Workshop: Reopen Locally` in the Command Palette.
The window reopens the project directory on the host;
the workshop keeps running.

To go back,
click :guilabel:`Reopen in Workshop` on the :samp:`dev` row.
The workshop is already running,
so the window reconnects within seconds.

To free the container's resources,
right-click the :samp:`dev` row
and choose :guilabel:`Turn Off…`.
After a confirmation,
this removes the container
together with any data stored in its default bind mounts;
your project directory on the host is not affected.
The definition stays,
so the next :guilabel:`Reopen in Workshop` launches the workshop afresh.

.. figure:: /images/vscode/turn-off-dialog.png
   :align: center
   :alt: A dialog asking Turn off "dev"? and explaining that the container
         and any data in the default bind mounts will be removed

   The Turn Off confirmation

.. note::

   You can also connect without the extension.
   Find the workshop's hostname with :command:`workshop info dev`
   and use it with Remote - SSH's own
   :guilabel:`Remote-SSH: Connect to Host…` command.


.. _how_vscode_commands:

Match actions to commands
-------------------------

Workshop lifecycle operations in the Workshops view use
the same :program:`workshopd` daemon as the :program:`workshop` command,
so the workshop's state is also visible from a terminal.
Editor-only actions operate on the VS Code window or its files instead:

.. list-table::
   :header-rows: 1
   :widths: 30 70

   * - Action
     - Command-line equivalent

   * - :guilabel:`Add New Workshop`
     - :command:`workshop init`, run by the wizard itself;
       the exact command line appears in the :guilabel:`Workshop` output channel

   * - :guilabel:`Reopen in Workshop`
     - :command:`workshop launch` for a workshop that was never launched,
       :command:`workshop start` for a stopped one,
       then an SSH connection to the workshop's hostname

   * - :guilabel:`Refresh and Reopen`
     - :command:`workshop refresh --wait-on-error`,
       then the SSH connection again

   * - :guilabel:`Continue Refresh`, :guilabel:`Abort Refresh`
     - :command:`workshop refresh --continue`
       or :command:`workshop refresh --abort`

   * - :guilabel:`Turn Off…`
     - :command:`workshop remove`

   * - The expanded row
     - :command:`workshop info`

   * - :guilabel:`Reopen Locally`, :guilabel:`Open Definition File`
     - No equivalent; these only act on the VS Code window


.. _how_vscode_troubleshoot:

Troubleshoot the extension
--------------------------

The Workshops view says Workshop isn't installed or running
   The extension can't reach the :program:`workshopd` daemon:

   .. figure:: /images/vscode/workshops-view-not-installed.png
      :align: center
      :alt: The Workshops view reporting that Workshop doesn't appear
            to be installed or running, with Install Workshop and Reload buttons

      The Workshops view without a reachable daemon

   Make sure :command:`workshop list` works in a terminal on the host,
   then click :guilabel:`Reload`.
   If the command fails as well,
   see :ref:`how_troubleshoot`.

The Workshops view says the installed Workshop is too old
   The extension requires |ws_markup| 0.9.5 or later.
   Upgrade with :command:`sudo snap refresh workshop`
   and click :guilabel:`Reload`.

The Workshops view is empty in Restricted Mode
   VS Code disables most extensions,
   including this one,
   in folders you haven't trusted.
   Either trust the folder,
   or allow the extension in untrusted workspaces
   by adding the following to your user settings:

   .. code-block:: json
      :caption: settings.json

      {
          "extensions.supportUntrustedWorkspaces": {
              "canonical.workshop": {
                  "supported": true
              }
          }
      }

The Workshop output shows diagnostic information
   The :guilabel:`Workshop` channel of the Output panel
   (:guilabel:`View` > :guilabel:`Output`)
   records extension diagnostics,
   including the exact :command:`workshop init` command the wizard runs
   and reported errors.
   Progress for other operations appears in notifications
   or task-specific error log views.


See also
--------

Explanation:

- :ref:`exp_arch_network`
- :ref:`exp_changes_tasks`
- :ref:`exp_workshop_hostname`

How-to guides:

- :ref:`how_debug_issues_workshops`
- :ref:`how_jetbrains_gateway`

Reference:

- :ref:`ref_workshop_info`
- :ref:`ref_workshop_init`
- :ref:`ref_workshop_refresh`

Tutorial:

- :ref:`tut_install`
