.. _ref_workshop_definition:

Workshop definition
===================

.. @artefact project

A project which defines a single workshop can store a definition file
named :file:`workshop.yaml` or :file:`.workshop.yaml`
in the project directory.


Filename convention
-------------------

.. @artefact project workshops
.. @artefact workshop name

When multiple workshops are defined,
their definition files must be stored in the :file:`.workshop/` subdirectory.
The workshop name must also match the file name
(without the :samp:`.yaml` extension).

Workshop names start with a lowercase letter
and may include only lowercase letters, digits or hyphens.


Structure
---------

The definition in the file is written in `YAML <https://yaml.org/>`__
and includes a number of mandatory and optional keys:

.. @artefact workshop base image
.. @artefact SDK

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 1 1 7

   * - Key
     - Value
     - Description

   * - :samp:`name` (required)
     - string
     - Workshop's name, used to reference the workshop itself.

       For workshops defined in the :file:`.workshop/` subdirectory,
       the definition file must have the same name
       (followed by :samp:`.yaml`).

   * - :samp:`base` (required)
     - string
     - Workshop's base image
       that provides the underlying OS capabilities.

       It can be :samp:`ubuntu@20.04`, :samp:`ubuntu@22.04`
       or :samp:`ubuntu@24.04`.

   * - :samp:`sdks`
     - object
     - List of individual SDKs
       from the SDK Store to include in the workshop.

       Each entry points to an existing SDK
       and specifies its retrieval channel.
       The SDKs are installed in the order they appear in this list;
       the exception is the system SDK which is always installed first.

   * - :samp:`connections`
     - array
     - List of connections made by the workshop;
       each links a plug to a slot.

       Any entry in :samp:`connections` must include
       a :samp:`plug` and a :samp:`slot` from the SDKs listed under :samp:`sdks`
       (the system SDK is always implicitly included).
       Both must be strings that reference a plug and a slot
       with the same interface,
       using the :samp:`<SDK>:<PLUG>` format.

   * - :samp:`scripts`
     - object
     - List of shell scripts to be used with :ref:`workshop run <ref_workshop_run>`.

       These are copied into the workshop
       before being executed by :command:`bash`.
       The options :samp:`errexit` and :samp:`pipefail`
       are set by default.


Each SDK is described with the following keys:

.. @artefact plug binding
.. @artefact $SDK

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 1 1 7

   * - Key
     - Value
     - Description

   * - :samp:`name` (required)
     - string
     - Name of an existing SDK,
       typically from the SDK Store.

       The :ref:`system SDK <ref_system_sdk>` is named :samp:`system`.

       :ref:`In-project SDKs <ref_in_project_sdk>` should be prefixed by :samp:`project-`.

   * - :samp:`channel`
     - string
     - SDK version to retrieve during
       :ref:`launch <ref_workshop_launch>`
       and
       :ref:`refresh <ref_workshop_refresh>`
       operations.

       It uses a
       `snap-like format <https://snapcraft.io/docs/channels>`__
       of :samp:`<TRACK>/<RISK>`
       without the :samp:`<BRANCH>` part.

       Required for SDKs from the Store.

   * - :samp:`plugs`
     - object
     - Lists plug bindings or additional plug definitions under the SDK.

       - A plug binding must name an existing plug in the SDK
         and set a single :samp:`bind` attribute
         that references a different plug of the same interface
         using the :samp:`<SDK>:<PLUG>` format.

       - A plug definition must specify the :samp:`interface`
         and the relevant attributes (described below).

   * - :samp:`slots`
     - object
     - Defines additional slots under the SDK;
       each entry must specify the :samp:`interface`
       and the relevant attributes (described below).


.. _ref_system_sdk:

System SDK
~~~~~~~~~~

.. @artefact system SDK

The system SDK is built into every workshop
to expose resources provided by the host system in a consistent way.
It's not available in the SDK Store,
so :samp:`channel` isn't relevant and can be omitted.

Technically, the system SDK is of :samp:`system` type,
whereas all other SDKs are of :samp:`regular` type,
but this detail isn't exposed in the definition files.

Several interfaces expose resources that are host-based and singular by nature;
the system SDK has default eponymous slots for these interfaces:
:samp:`system:camera`, :samp:`system:desktop`, :samp:`system:gpu`,
:samp:`system:mount`, and :samp:`system:ssh-agent`.
No other SDKs can declare slots for these interfaces, except for :samp:`mount`.
The :samp:`system:mount` slot is still unique
because it's the only one that provides access to the *host* file system,
whereas slots under regular SDKs only expose locations in the workshop.

If additional slots for interfaces like :samp:`tunnel` or :samp:`mount`
are defined for the system SDK,
they won't be auto-connected at launch or refresh,
largely due to security considerations,
because the system SDK exposes sensitive host system resources.
To the contrary, plugs added under the system SDK can be auto-connected
because they expose workshop internals.


.. _ref_in_project_sdk:

In-project SDKs
~~~~~~~~~~~~~~~

.. @artefact in-project SDK

Projects can define their own SDKs
to configure the workshop in a project-specific way.
These SDKs are defined by files named like :file:`.workshop/<NAME>/sdk.yaml`,
relative to the project directory.
Accordingly, SDK hooks are stored under :file:`.workshop/<NAME>/hooks/`.

Workshops within the project consume these SDKs
using names like :samp:`project-<NAME>`;
a :samp:`channel` is not required in this case.


Camera interface
~~~~~~~~~~~~~~~~

.. @artefact camera interface

Camera interface plugs must be named :samp:`camera`
and can't belong to the :ref:`system SDK <ref_system_sdk>`.
They have no attributes.

The only camera interface slot is :samp:`system:camera`.


Desktop interface
~~~~~~~~~~~~~~~~~

.. @artefact desktop interface

Desktop interface plugs must be named :samp:`desktop`
and can't belong to the :ref:`system SDK <ref_system_sdk>`.
They have no attributes.

The only desktop interface slot is :samp:`system:desktop`.


GPU interface
~~~~~~~~~~~~~

.. @artefact GPU interface

GPU interface plugs must be named :samp:`gpu`
and can't belong to the :ref:`system SDK <ref_system_sdk>`.
They have no attributes.

The only GPU interface slot is :samp:`system:gpu`.


Mount interface
~~~~~~~~~~~~~~~

.. @artefact mount interface

Mount interface plugs can't belong to the :ref:`system SDK <ref_system_sdk>`.
They are described by the following attributes:

.. @artefact mount interface attributes

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`workshop-target` (required)
     - string
     - A path inside the workshop
       to be used as the plug's target directory.

   * - :samp:`read-only`
     - Boolean
     - Whether the target directory should be read-only.


The only mount interface slot in the :ref:`system SDK <ref_system_sdk>`
is :samp:`system:mount`.
It has a single dynamic attribute named :samp:`host-source`,
which can be only configured at :ref:`remount <ref_workshop_remount>`.

Regular SDKs can declare additional mount interface slots.
They are described by the following attributes:

.. @artefact mount interface attributes
.. @artefact $SDK

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`workshop-source` (required)
     - string
     - A path inside the workshop
       to be used as the slot's source directory;
       :file:`/project` or :envvar:`$SDK`-based paths can be used;
       :envvar:`$SDK` expands into the SDK's installation path in the workshop.


SSH interface
~~~~~~~~~~~~~

.. @artefact SSH interface

SSH interface plugs must be named :samp:`ssh-agent`
and can't belong to the :ref:`system SDK <ref_system_sdk>`.
They have no attributes.

The only SSH interface slot is :samp:`system:ssh-agent`.


Tunnel interface
~~~~~~~~~~~~~~~~

.. @artefact tunnel interface

Tunnel interface plugs and slots are described by the following attributes:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`endpoint`
     - string
     - A network address or Unix domain socket
       to be used as one end of the tunnel.


Endpoints are formatted as follows:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 7

   * - Type
     - Format

   * - Endpoint
     - :samp:`<ADDRESS>/<PROTOCOL>` for network endpoints.
       May be shortened to :samp:`<ADDRESS>` or :samp:`<PROTOCOL>`

       :samp:`<PATH>` or :samp:`@<STRING>` for Unix domain sockets.

   * - Address
     - :samp:`<HOST>:<PORT>`.
       May be shortened to :samp:`<HOST>` or :samp:`<PORT>`.

   * - Protocol
     - Either :samp:`tcp` or :samp:`udp`.
       The default is :samp:`tcp`.

   * - Host
     - An IPv4 or IPv6 address.

       If a port is supplied,
       IPv6 addresses must be enclosed in square brackets.

       Supported aliases: :samp:`localhost`, :samp:`ip6-localhost` and :samp:`ip6-loopback`.

       The default is :samp:`localhost`.

   * - Port
     - A TCP or UDP port number (1–65535).

       May be omitted,
       but only on one side of a connection.
       For such connections,
       both sides use the same port.

       For security reasons,
       tunnel interface plugs in the system SDK
       cannot use privileged ports (1–1023).

   * - Path
     - An absolute path to a Unix domain socket.

       :envvar:`$HOME` expands into the user's home directory and
       :envvar:`$XDG_RUNTIME_DIR` expands into the user runtime directory
       (e.g. :file:`/run/user/1000`).

       For security reasons,
       tunnel interface plugs in the system SDK
       cannot listen on sockets outside these two directories.

   * - String
     - An abstract socket name.


The default :samp:`endpoint` is the default network address (:samp:`localhost/tcp`).

Endpoints which start with :samp:`[` or :samp:`@`
need to be quoted in YAML:

.. code-block:: yaml

   endpoint: '[::1]:8080/tcp'
   endpoint: '@abstract.sock'


JSON Schema
-----------

.. The schema can be exported from internal/workshop/workshop_file.go

The following
`JSON Schema`
formalizes the description above:

.. @artefact workshop schema

.. dropdown:: Workshop definition schema

   .. literalinclude:: schema.json
      :language: json


Examples
--------

This YAML file defines a :samp:`golang` workshop
with a single :samp:`go` SDK
from the :samp:`latest/stable` channel,
and some useful scripts:

.. code-block:: yaml
   :caption: .workshop/golang.yaml

   name: golang
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: latest/stable
   scripts:
     lint: |
       go vet
       golangci-lint run
     tests: go test "$@"


This YAML file defines a :samp:`go-dev` workshop
that uses the :samp:`go` SDK from the Store
and an :ref:`in-project SDK <ref_in_project_sdk>` named :samp:`tunnel`;
the :samp:`data` plug defined by the :samp:`tunnel` SDK
is bound to the :samp:`mod-cache` plug of the :samp:`go` SDK:

.. code-block:: yaml
   :caption: .workshop/go-dev.yaml

   name: go-dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: latest/candidate
     - name: project-tunnel
       plugs:
         data:
           bind: go:mod-cache


This YAML file, besides using the :samp:`tensorflow`, :samp:`imagenet` and :samp:`cuda` SDKs,
defines an additional slot under the :samp:`imagenet` SDK, a plug under :samp:`tensorflow`
and two connections:

- One that connects the :samp:`tensorflow:images` plug
  to the newly defined :samp:`imagenet:images` slot.

- Another that connects the :samp:`tensorflow:cuda` plug
  to the pre-existing :samp:`cuda:libs`.

.. code-block:: yaml
   :caption: .workshop/digits-cuda.yaml

   base: ubuntu@22.04
   name: digits-cuda
   sdks:
     - name: tensorflow
       channel: latest/stable
       plugs:
         cuda:
           interface: mount
           workshop-target: /usr/local/cuda/lib64
     - name: imagenet
       channel: latest/stable
       slots:
         images:
           interface: mount
           workshop-source: $SDK/images
     - name: cuda
       channel: latest/stable
   connections:
     - plug: tensorflow:cuda
       slot: cuda:libs
     - plug: tensorflow:images
       slot: imagenet:images


See also
--------

Explanation:

- :ref:`exp_base`
- :ref:`exp_sdks`
- :ref:`exp_system_sdk`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_workshop_info`
