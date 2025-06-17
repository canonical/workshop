.. _how_sdkcraft:

How to craft SDKs with |sdk_markup|
===================================

.. @artefact sdkcraft (CLI)
.. @artefact SDK
.. @artefact interface

This is a practical how-to guide
that takes you on a tour
of the essential |sdk_markup| activities.

Here, you will initialize, define, pack, and publish an :ref:`SDK <exp_sdks>`:
a set of hooks, interfaces, and parts that is bundled into a single package,
suitable for use with |sdk_markup|, the user-oriented CLI utility.
The commands you're about to run
cover most of your daily needs with |sdk_markup|.

For more details, see the
:ref:`reference <ref_index>` and :ref:`explanation <exp_index>` sections.


Check the prerequisites
-----------------------

|sdk_markup| relies on
`LXD 6.3+ <https://canonical.com/lxd>`_
for low-level operation,
using its
`REST API <https://documentation.ubuntu.com/lxd/latest/restapi_landing/>`_
to craft the SDKs.

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


Install |sdk_markup|
--------------------

Download the latest snap from |sdk_markup|'s
`Releases <https://github.com/canonical/sdkcraft/releases/>`__
page on GitHub.

Install it using the
`--dangerous <https://snapcraft.io/docs/install-modes>`_
and
`--classic <https://snapcraft.io/docs/install-modes>`_
options, for example:

.. code-block:: console

   $ sudo snap install --dangerous --classic ./sdkcraft_0.1.2_amd64.snap


The snap installs the :program:`sdkcraft` CLI tool.
Make sure it runs:

.. code-block:: console

   $ sdkcraft --help


.. _how_sdkcraft_init:

Initialize the SDK
------------------

Once you have installed |sdk_markup|,
use it to initialize, define and pack your first :ref:`SDK <exp_sdks>`.
Here, we'll build an SDK that installs a version of Go in the workshop.

First, create a directory named :file:`go/`:

.. code-block:: console

   $ mkdir go/


.. @artefact SDK definition

It will contain your :ref:`SDK definition <exp_sdk_definition>`
and other source files.

Next, browse to the SDK directory and initialize it:

.. code-block:: console

   $ cd go/
   $ sdkcraft init


This command creates a template definition file
named :file:`sdk.yaml`;
although it's almost empty,
it can already be :ref:`built <how_sdkcraft_build_sdk>`.

However, let's take a few extra steps
to explore what goes into an SDK.


Update metadata
---------------

Update the metadata in :file:`sdk.yaml`,
adjusting its :samp:`name`, :samp:`summary` and :samp:`description`:

.. code-block:: yaml
   :caption: sdk.yaml
   :emphasize-lines: 1,4-6

   name: go
   base: ubuntu@24.04
   version: '0.1'
   summary: Go SDK
   description: |
     This is my Go SDK description.
   license: GPL-3.0
   platforms:
     amd64:

   parts:
     my-part:
       plugin: nil


.. _how_sdkcraft_parts:

Define parts
------------

|sdk_markup| leverages the :ref:`parts mechanism <exp_sdk_parts>`
to obtain data from different sources, process it in various ways,
and prepare an SDK package for publishing.

In our example, the :samp:`parts` section of the definition can be used as is:

.. code-block:: yaml
   :caption: sdk.yaml

   # ...
   parts:
     my-part:
       plugin: nil


For in-depth details,
refer to the `Parts
<https://canonical-craft-parts.readthedocs-hosted.com/en/latest/common/craft-parts/explanation/parts/>`_
section in Craft Parts documentation.


.. _how_sdkcraft_mount_interface:

Add interface plugs
-------------------

.. @artefact interface plug

In |sdk_markup|,
:ref:`interfaces <exp_interfaces>` provide a controllable way
of exposing the resources of the host system to the workshops,
and you can use them in a variety of ways
to extend the functionality of your SDK.

Suppose you want to preserve the Go module cache
when a workshop using your SDK is rebuilt from scratch, or *refreshed*.
You can use a :ref:`mount interface <exp_mount_interface>` plug for that:
it mounts a host directory to a target directory in the workshop,
so that the files remain on the host.

Open :file:`sdk.yaml` again
and add a plug named :samp:`mod-cache` to the :samp:`plugs` section:

.. code-block:: yaml
   :caption: sdk.yaml
   :emphasize-lines: 15-18

   name: go
   base: ubuntu@24.04
   version: '0.1'
   summary: Go SDK
   description: |
     This is my Go SDK description.
   license: GPL-3.0
   platforms:
     amd64:

   parts:
     my-part:
       plugin: nil

   plugs:
     mod-cache:
       interface: mount
       workshop-target: /home/workshop/go/pkg/mod


Now, when a workshop using this SDK will be started,
|ws_markup| will map the plug's :samp:`target` in the workshop
to a host directory that will be automatically created
and maintained between refresh operations.

.. note::

   You can't explicitly set the *host* directory here;
   this prevents SDKs from accessing any arbitrary data on the host file system.
   However, users who add your SDK to their workshops
   will be able to remount the plug elsewhere at run-time.


Add hooks
---------

.. @artefact SDK hook

To prepare the SDK for use,
add the :ref:`hooks <exp_sdk_hooks>`
that run at different stages of the workshop's life cycle,
preparing the SDK for use or preserving its state during updates.

Under :file:`go/`,
create a subdirectory
named :file:`hooks/`:

.. code-block:: console

   $ mkdir hooks/
   $ cd hooks/

This directory stores all the hooks for an SDK.


Build: setup base and project
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. @artefact setup-base
.. @artefact setup-project

Under :file:`go/hooks/`,
create a file
named :file:`setup-base`:

.. code-block:: shell
   :caption: setup-base

   snap install --classic go
   
   # Create a mod cache directory to be mounted using the mount interface
   cache=$(sudo -u workshop -- go env GOMODCACHE)
   sudo -u workshop -- mkdir -p "$cache"


It runs when the workshop is launched or refreshed,
installing system packages and preparing the workshop for use.

.. note::

   For workshops,
   :command:`apt` is configured to exclude recommended or suggested packages
   and answer 'yes' to all confirmation prompts.


In the same directory,
create a file named :file:`setup-project`:

.. code-block:: shell
   :caption: setup-project

   go mod download

   echo "PATH=/home/workshop/go/bin:$PATH" | tee -a /home/workshop/.profile


It runs after :file:`setup-base`,
once the project directory is mounted
and interfaces are connected.
This can be used to install project-specific packages
or setup the :samp:`workshop` user's environment.


Persist: save and restore
~~~~~~~~~~~~~~~~~~~~~~~~~

.. @artefact restore-state
.. @artefact save-state

Also under :file:`go/hooks/`,
create two files
named :file:`save-state` and :file:`restore-state`:

.. code-block:: shell
   :caption: save-state

   sudo -u workshop go env -changed | sed "s/='\(.*\)'/=\1/" | tee "$SDK_STATE_DIR"/env-vars


.. code-block:: shell
   :caption: restore-state

   while IFS='=' read -r key value; do
     sudo -u workshop go env -w "$key=$value"
   done < "$SDK_STATE_DIR"/env-vars


During a :command:`workshop refresh` operation:

- The :file:`save-state` hook runs *before* the workshop is refreshed,
  saving the state of the SDK.

- The :file:`restore-state` hook recovers the state
  *after* the workshop has been successfully updated.

- Both hooks use :envvar:`$SDK_STATE_DIR`
  for the workshop directory where the state is stored.


.. important::

   The SDK is also refreshed as a part of the workshop,
   so any breaking changes in its save-restore logic will cause an error.


Report: check health
~~~~~~~~~~~~~~~~~~~~

.. @artefact check-health

Finally, create a hook named :file:`check-health`:

.. @artefact workshopctl

.. code-block:: shell
   :caption: check-health

   if go version > /dev/null 2>&1; then
     workshopctl set-health okay
   else
     workshopctl set-health --code=installation-fails error "Go installation fails"
   fi


It checks whether the Go installation is functional.
If it is, the health is set to :samp:`okay`
using the :ref:`workshopctl set-health <exp_workshopctl>` command;
otherwise, a similar command reports an error.

In general, the hook should set the health to :samp:`okay`
and return a zero code if everything is fine.
To signal an error, set the health to :samp:`error`
or return a non-zero code.

You can also set the health to :samp:`waiting`
to signal that the hook should be retried for a few seconds.
Unless the hook sets the health to a different value during such a retry,
the health is eventually set to :samp:`error` automatically.


.. _how_sdkcraft_build_sdk:

Build and pack
--------------

Under :file:`go/`, run:

.. code-block:: console

   $ sdkcraft


.. @artefact SDK part

This builds all :ref:`SDK parts <exp_sdk_parts>`
defined in the :file:`sdk.yaml` file,
e.g. pulling source code, applying patches, configuring and compiling it
according to the part definition.

.. note::

   For a detailed explanation of the build process,
   see the respective Craft Parts
   `documentation section
   <https://canonical-craft-parts.readthedocs-hosted.com/en/latest/common/craft-parts/explanation/lifecycle/>`_.


Optionally, you can clean the build cache before a build attempt:

.. code-block:: console

   $ sdkcraft clean && sdkcraft


.. @artefact SDK metadata

Ran without arguments,
:command:`sdkcraft` builds and packs the SDK into the :file:`go.sdk` file,
which contains the build artifacts from the previous step
along with SDK metadata, hooks and other components.


.. _how_sdkcraft_publish:

Publish the SDK
---------------

.. @artefact SDK Store

When an SDK is ready and packed,
you need to publish it to the SDK Store
for use with |ws_markup|:

.. code-block:: console

   $ sdkcraft.publish ./go.sdk latest/beta


This publishes the newly created SDK
under the :samp:`latest/beta` channel in the SDK Store.


Use the SDK
-----------

The resulting SDK can be accessed by |ws_markup| as follows:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 2,4,5

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: go
       channel: latest/beta


Mind that the :samp:`base` of the workshop must match the SDK :samp:`base`.

.. note::

   Currently, you can't use unpublished SDKs in a workshop.
   However, the :ref:`sketch SDK <how_sketch>` and in-project SDKs provide
   a subset of |sdk_markup|'s functionality.


This was the last step of the guide;
you are now familiar with the basic operations |sdk_markup| provides
and have had your first taste of what it can do for you.

See also
--------

Reference:

- :ref:`ref_workshopctl`
