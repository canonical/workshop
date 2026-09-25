.. _how_use_virtual_machines:

.. meta::
   :description: How-to guide on opting into the experimental virtual machine
                 runtime for workshops, covering the experimental snap
                 setting and daemon restart, both ways to declare the
                 lxd-vm runtime, replacing the confinement key, launching
                 and verifying the workshop, and the limitations that
                 separate it from a container.

How to use virtual machines
===========================

.. @artefact workshop init
.. @artefact workshop info

A workshop runs in an LXD container unless its definition says otherwise.
|ws_markup| can instead run it as a full virtual machine
with a kernel of its own,
which draws a harder boundary than a container can.
Reach for a virtual machine when you want that boundary,
or when the workshop itself has to run LXD containers and virtual machines.
The boundary stops at the kernel:
the project directory, including its :file:`.git` directory,
is still mounted read-write,
and outbound network access is still open,
so a virtual machine doesn't protect the project
from a coding agent that works in it.

.. warning::

   The virtual machine runtime is experimental.
   Containers remain the default and the supported choice.
   A virtual machine consumes more memory and disk than a container,
   and it does not support every feature a container does;
   the :ref:`how_use_virtual_machines_limitations` section lists what it skips.
   Opt in only when you need the stronger boundary.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- A |ws_markup| installation that supports the :samp:`runtime` key.
- An LXD installation that can run virtual machines,
  on a host with hardware virtualization available.
- A workshop you have not launched yet.
  The runtime is fixed when a workshop is launched,
  so an already running container workshop cannot be converted.

Check that |ws_markup| lists the runtimes it supports:

.. code-block:: console

   $ workshop init --help

     ...
     The supported runtimes are lxd-container (the default) and lxd-vm.
     ...


Check that LXD is installed:

.. code-block:: console

   $ lxc version

     Client version: 6.9
     Server version: 6.9


Requirements for SDKs
~~~~~~~~~~~~~~~~~~~~~

A virtual machine workshop that carries SDKs
needs one more capability from LXD:
the ability to mount shifted disks into a virtual machine.
Query LXD for it:

.. code-block:: console

   $ lxc query /1.0/metadata/configuration | jq -r '.configs["device-disk"]["device-conf"].keys[] | select(has("shift")) | .shift.condition'

     container


A result of :samp:`container` means this LXD offers shifted mounts
to containers only,
so a virtual machine workshop on it must declare no SDKs.
Install LXD from the :samp:`latest/edge` channel to get the capability.

A workshop that declares no SDKs is unaffected by this capability
and needs nothing beyond a working virtual machine.


Opt in to virtual machines
--------------------------

.. @artefact workshop launch

Until you opt in,
|ws_markup| refuses to launch a virtual machine workshop
and reports the two commands that change that:

.. code-block:: console

   $ workshop launch <NAME>

     error: cannot launch "<NAME>": runtime "lxd-vm" is experimental
     To opt in: "sudo snap set workshop workshop.experimental-vms=1 && sudo snap restart workshop.workshopd"


Run both of them.
The restart is part of the opt-in, not a follow-up you can defer:

.. code-block:: console

   $ sudo snap set workshop workshop.experimental-vms=1
   $ sudo snap restart workshop.workshopd

     Restarted.


Create a virtual machine workshop
---------------------------------

Generate a definition with the :option:`!--runtime` flag:

.. code-block:: console

   $ workshop init <NAME> --runtime lxd-vm

     "<NAME>" workshop created at /home/user/my-project/.workshop/<NAME>.yaml


The generated definition carries the runtime:

.. code-block:: yaml
   :caption: .workshop/<NAME>.yaml
   :emphasize-lines: 3

   name: <NAME>
   base: ubuntu@24.04
   runtime: lxd-vm


Add the same key by hand to a definition you already wrote.
:samp:`runtime` accepts :samp:`lxd-container` and :samp:`lxd-vm`;
omitting it selects :samp:`lxd-container`.


Replace the confinement key
---------------------------

|ws_markup| does not read a :samp:`confinement` key.
A definition that still declares :samp:`confinement: virtual-machine`
launches a container, without an error,
so replace that line with :samp:`runtime: lxd-vm`.

A virtual machine workshop that an earlier |ws_markup| version
launched from such a definition
reports :samp:`runtime: lxd-container` in :command:`workshop info`,
and refreshing it against the corrected definition fails:

.. code-block:: console

   $ workshop refresh <NAME>

     error: cannot refresh "<NAME>": cannot refresh "<NAME>": runtime changed from "lxd-container" to "lxd-vm"


Remove the workshop with :command:`workshop remove`,
then launch it again as described in :ref:`how_use_virtual_machines_launch`.


.. _how_use_virtual_machines_launch:

Launch and verify
-----------------

Launch the workshop the same way as any other:

.. code-block:: console

   $ workshop launch <NAME>

     "<NAME>" launched


A virtual machine boots from a different base image than a container,
so the first launch on a given base downloads that image.

Confirm what you got:

.. code-block:: console

   $ workshop info <NAME>

     name:      <NAME>
     base:      ubuntu@24.04
     project:   ~/my-project
     hostname:  <NAME>.my-project.wp
     status:    ready
     runtime:   lxd-vm
     notes:     --


The :samp:`runtime` line is the confirmation:
a container workshop reports :samp:`runtime: lxd-container` instead.


Manage the workshop
-------------------

Stopping, starting, and removing a virtual machine workshop
use the same commands as a container:

.. code-block:: console

   $ workshop stop <NAME>

     "<NAME>" stopped


.. code-block:: console

   $ workshop start <NAME>

     "<NAME>" started


.. code-block:: console

   $ workshop remove <NAME>

     "<NAME>" removed


A running virtual machine workshop also starts again on its own
after the host reboots,
and it is reachable over SSH by hostname,
in both cases exactly as a container workshop is.
See :ref:`how_use_multiple_workshops_networking`
for reaching a workshop by hostname.


.. _how_use_virtual_machines_limitations:

Limitations
-----------

.. @artefact workshop refresh

Virtual machine workshops are not at parity with containers.
The differences below change what you can do with one.


The runtime is fixed at launch
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Editing :samp:`runtime` in the definition of a launched workshop
and refreshing it fails:

.. code-block:: console

   $ workshop refresh <NAME>

     error: cannot refresh "<NAME>": cannot refresh "<NAME>": runtime changed from "lxd-vm" to "lxd-container"


Remove the workshop and launch it again to change the runtime.


SDK support depends on LXD
~~~~~~~~~~~~~~~~~~~~~~~~~~

Where LXD offers shifted mounts to containers only,
as described in the prerequisites,
a virtual machine workshop that declares an SDK is refused:

.. code-block:: console

   $ workshop refresh <NAME>

     error: cannot refresh "<NAME>": cannot refresh "<NAME>": SDKs are currently unavailable for virtual machines


SDK interfaces are not connected automatically
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Launching a container connects the SDK plugs that qualify for it;
launching a virtual machine skips that step.
Connect the plugs you need with :command:`workshop connect`.
Refreshing a virtual machine workshop does not restore connections either,
so reconnect them after a refresh.


SDK health checks do not run
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The health check that a container runs
after a launch and after a refresh
is skipped for a virtual machine,
so :command:`workshop info` reports no SDK health notes for one.


See also
--------

Explanation:

- :ref:`exp_arch_lxd_backend`
- :ref:`exp_base`
- :ref:`exp_workshop_definition`


How-to guides:

- :ref:`how_use_multiple_workshops`


Reference:

- :ref:`ref_ai_agents`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_definition`
- :ref:`ref_workshop_info`
- :ref:`ref_workshop_init`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_remove`
- :ref:`ref_workshop_start`
- :ref:`ref_workshop_stop`
