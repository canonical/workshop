.. _how_use_multiple_workshops:

.. meta::
   :description: How-to guide on using multiple workshops in a single project
                 for modular isolation, covering setup, management,
                 sharing in-project SDKs, and cross-workshop networking
                 via the tunnel interface.

How to use multiple workshops in a project
==========================================

.. @tests in tests/docs-how-to/use-multiple-workshops/task.yaml

.. @artefact project workshops
.. @artefact workshop definition

A project may require different toolchains for different components,
such as a Go backend and a Node.js frontend.
Instead of putting everything into a single workshop definition,
you can define multiple workshops in the same project directory.
Each workshop is an independent environment
with its own base image, SDKs, and actions;
at the same time, the workshops share a single project directory
mounted at :file:`/project/`.


Set up definitions
------------------

.. @artefact workshop name

When a project uses multiple workshops,
store their definitions in the :file:`.workshop/` subdirectory
instead of a single :file:`workshop.yaml` in the project root.
Each file must be named after its workshop,
so the :samp:`name` field matches the file name
(without the :samp:`.yaml` extension).

Here is a project layout
with two workshop definitions
and a shared in-project SDK:

.. code-block:: none

   my-project/
   ├── .workshop/
   │   ├── frontend.yaml
   │   ├── backend.yaml
   │   └── common-tools/
   │       └── sdk.yaml
   ├── web/
   └── api/


The :samp:`frontend` workshop uses the :samp:`node` SDK
for the browser-facing part of the project:

.. code-block:: yaml
   :caption: .workshop/frontend.yaml

   name: frontend
   base: ubuntu@24.04
   sdks:
     - name: node
       channel: 24
   actions:
     build: |
       npm run build


The :samp:`backend` workshop uses the :samp:`go` SDK
for the server-side code:

.. code-block:: yaml
   :caption: .workshop/backend.yaml

   name: backend
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: 1.26
   actions:
     test: |
       go test ./...


Each workshop can use a different base image,
a different set of SDKs,
and its own actions,
all while sharing the project directory.

What's more, you can share in-project SDKs across workshops,
as described in a section below.

.. note::

   You cannot mix a root-level :file:`workshop.yaml`
   with files in :file:`.workshop/`.
   If |ws_markup| finds both,
   it reports an error.


Launch and manage workshops
---------------------------

.. @artefact workshop launch

Launch both workshops at once:

.. code-block:: console

   $ workshop launch frontend backend


When a project has multiple workshops,
the workshop name is required in every command;
you cannot omit it as you would with a single-workshop project.

Check the status of both workshops:

.. @artefact workshop list

.. code-block:: console

   $ workshop list

     WORKSHOP  STATUS  NOTES
     frontend  Ready   -
     backend   Ready   -


Run an action in a specific workshop:

.. code-block:: console

   $ workshop run frontend -- build
   $ workshop run backend -- test


Shell into one of the workshops:

.. code-block:: console

   $ workshop shell backend


Execute a one-off command:

.. code-block:: console

   $ workshop exec frontend -- node --version


Stop and start workshops independently:

.. code-block:: console

   $ workshop stop frontend
   $ workshop start frontend


Or stop both at once:

.. code-block:: console

   $ workshop stop frontend backend


To see the status of workshops in the current project,
use the :command:`workshop list` command without arguments:

.. code-block:: console

   $ workshop list

     WORKSHOP  STATUS  NOTES
     frontend  Ready   -
     backend   Ready   -


To see the status of workshops across all projects on the system,
use the :option:`!--global` flag:

.. code-block:: console

   $ workshop list --global

     PROJECT                   WORKSHOP  STATUS  NOTES
     /home/user/my-project     frontend  Ready   -
     /home/user/my-project     backend   Ready   -
     /home/user/other-project  dev       Ready   -


When you no longer need the workshops, remove them:

.. code-block:: console

   $ workshop remove frontend backend


Share in-project tools
----------------------

.. @artefact in-project SDK

If multiple workshops need the same custom tooling,
define an in-project SDK
rather than duplicating hooks or configuration.
In-project SDKs are stored
in subdirectories of :file:`.workshop/`
and referenced with the :samp:`project-` prefix.

Both workshops can then include it:

.. code-block:: yaml
   :caption: .workshop/frontend.yaml
   :emphasize-lines: 6

   name: frontend
   base: ubuntu@24.04
   sdks:
     - name: node
       channel: 24
     - name: project-common-tools


.. code-block:: yaml
   :caption: .workshop/backend.yaml
   :emphasize-lines: 6

   name: backend
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: 1.26
     - name: project-common-tools


After adding the SDK references,
refresh the workshops to pick up the change:

.. code-block:: console

   $ workshop refresh frontend backend


Cross-workshop networking
-------------------------

.. @artefact tunnel interface

You cannot connect a plug in one workshop to a slot in another;
:command:`workshop connect` rejects such attempts.
However, all workshops on the same machine share a common host,
and the tunnel interface can bridge through it.

The idea is to compose two independent tunnels:
one that exposes a service from the backend workshop to the host,
and another that lets the frontend workshop reach that host port.
This is different from a regular intraworkshop connection,
where a single tunnel links a plug to a slot inside the same workshop.
Here, the host sits in the middle,
and each workshop configures its own half of the bridge.

The backend workshop exposes its API on the host
by pairing a :samp:`system` plug with a regular SDK slot:

.. code-block:: yaml
   :caption: .workshop/backend.yaml
   :emphasize-lines: 5-8, 10-13

   name: backend
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: 1.26
       slots:
         api:
           interface: tunnel
           endpoint: localhost:8080    # service inside the workshop
     - name: system
       plugs:
         api:
           interface: tunnel
           endpoint: localhost:8080    # port on the host


The frontend workshop reaches the host port
by pairing a regular SDK plug with a :samp:`system` slot:

.. code-block:: yaml
   :caption: .workshop/frontend.yaml
   :emphasize-lines: 5-8, 10-13

   name: frontend
   base: ubuntu@24.04
   sdks:
     - name: node
       channel: 24
       plugs:
         api:
           interface: tunnel
           endpoint: localhost:8080    # where the client connects
     - name: system
       slots:
         api:
           interface: tunnel
           endpoint: localhost:8080    # host-side port (bridged from backend)


Launch both workshops.
The backend tunnel auto-connects
because its plug is on the :samp:`system` SDK with a matching name,
but the frontend tunnel requires a manual step:

.. code-block:: console

   $ workshop launch frontend backend
   $ workshop connect frontend/node:api


Verify the connection:

.. code-block:: console

   $ workshop connections frontend

     INTERFACE  PLUG                   SLOT                   NOTES
     tunnel     frontend/node:api      frontend/system:api    manual


After this, any service listening on port 8080 inside the backend workshop
is reachable at :samp:`localhost:8080` from within the frontend workshop.


.. note::

   The host port must be free before launching the backend workshop,
   or the tunnel will fail to activate.
   If you need several cross-workshop tunnels,
   use a different port for each.
   See :ref:`how_forward_ports` for tunnel basics and troubleshooting.


See also
--------

Explanation:

- :ref:`exp_in_project_sdk`
- :ref:`exp_multi_workshop_patterns`
- :ref:`exp_projects`
- :ref:`exp_tunnel_interface`
- :ref:`exp_workshop_definition`

How-to guides:

- :ref:`how_add_actions`
- :ref:`how_forward_ports`
- :ref:`how_git_workshops`
- :ref:`how_move_projects`

Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_exec`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_list`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_remove`
- :ref:`ref_workshop_run`
- :ref:`ref_workshop_shell`
- :ref:`ref_workshop_start`
- :ref:`ref_workshop_stop`
