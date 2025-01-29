.. _how_create_ros2_sdk:

How to design an SDK
====================

.. @artefact SDK

For a practical example of SDK design and layout,
let's see how an SDK for
`ROS 2
<https://docs.ros.org/en/jazzy/Tutorials/Beginner-Client-Libraries/Creating-A-Workspace/Creating-A-Workspace.html>`_
that we have published in our SDK Store
is structured.

.. note::

   This guide assumes that you already have |sdk_markup| installed
   and know how to use it; if needed, see the :ref:`tutorial <tutorial>` first.
   Also, our ROS 2 SDK is currently based on the :samp:`jazzy` distribution;
   adapt these steps for other distributions as needed.


Define the SDK
--------------

.. @artefact sdkcraft (CLI)
.. @artefact SDK definition

Here's the entire SDK definition:

.. literalinclude:: design-sdk/sdkcraft.yaml
   :language: yaml


Looks great, but what does it do?
Let's review the less trivial sections:

.. list-table::
   :widths: 1 3

   * - :samp:`platform`
     - The SDK targets both :samp:`amd64` and :samp:`arm64`.

   * - :samp:`parts`
     - This is essentially a stub;
       currently, the SDK doesn't rely on it.

       Also subject to change,
       some actions done with :ref:`hooks <how_ros2_sdk_hooks>`
       may move here.

   * - :samp:`plugs`
     - This section defines two mount plugs and a GPU plug.

       - The first mount plug, :samp:`ros-cache`, maps ROS 2 configuration
         to a host directory to preserve it between refreshes.

       - The second one, :samp:`colcon-artefacts`,
         is where the build artefacts will end up at run-time,
         so the build cache can be persisted and reused.

       - The GPU plug provides GPU pass-through for the SDK.


Summarily, the definition builds upon |sdk_markup|'s capabilities,
persisting the important reusable parts of the setup on the host
and making its GPU capabilities directly available.

.. @artefact SDK hook

However,
the SDK should actually make use of the directories defined in mount plugs.
For :samp:`ros-cache`,
this occurs automatically because it's a default,
but we need to explicitly tip the SDK
to place the build under the :samp:`colcon-artefacts` target.
Currently, this is achieved with an SDK hook.


.. _how_ros2_sdk_hooks:

Define the hooks
----------------

.. @artefact project

This design doesn't require preserving state between refreshes.
As seen above, the content is cached on the host,
so we only need to define the :samp:`setup-base` hook
to install the SDK in the workshop.

The hook is available as a :download:`file <design-sdk/setup-base>`;
in this section, let's focus on its major portions and what they do.
Besides installing the prerequisites, the hook does two important things:

- Points the build configuration to the directory set for :samp:`colcon-artefacts`
- Looks up project dependencies in the :samp:`/project/` directory,
  assuming the project sources are already mapped there,
  and installs them automatically

Both aspects simplify reuse and reentry for SDK users;
we'll discuss how they work in detail below.


Path variables
~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [path-variables-start]
   :end-before: [path-variables-end]


Defines various environment-specific variables,
including paths for the workspace, configuration files,
and the ROS 2 distribution (:samp:`jazzy`).


Package management setup
~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [apt-update-start]
   :end-before: [apt-update-end]


Updates the package list and ensures necessary tools
(:program:`software-properties-common` and :program:`curl`)
are installed and the :samp:`universe` repository is enabled.

With this, we can enable the specific repository we need.

Setup ROS 2 GPG key and repository
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [ros2-repo-start]
   :end-before: [ros2-repo-end]


Downloads the ROS 2 GPG key and adds the ROS 2 repository
to the sources list for package management;
this is done in a manner typical of Ubuntu-based installations
(mind that we target :samp:`ubuntu@24.04` as :samp:`base`).


Install ROS 2 development tools
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [ros2-devtools-start]
   :end-before: [ros2-devtools-end]


Using the repository configured earlier,
this installs ROS 2 development tools and additional :program:`colcon` tools
for building and managing packages (this time, in the sense of ROS 2).


Setup minimal ROS 2 workspace
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [ros2-workspace-start]
   :end-before: [ros2-workspace-end]


Installs minimal workspace packages and tools
for running and launching ROS 2 nodes.


Update environment for ROS 2 and :program:`colcon`
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [bashrc-update-start]
   :end-before: [bashrc-update-end]


Adds lines to the :file:`.profile` and :file:`.bashrc` files
to set up the ROS 2 environment and auto-completion for :program:`colcon`,
ensuring they are loaded in every new shell session
(|ws_markup| uses :program:`bash` by default).


Configure :program:`colcon` defaults
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [colcon-defaults-start]
   :end-before: [colcon-defaults-end]


Creates directories and a default configuration file for :program:`colcon`
with build, install, and log file paths.

.. important::

   This is where the :samp:`colcon-artefacts` plug from :file:`sdkcraft.yaml`
   comes into play;
   the configuration points the build actions there instead of the default path.


Add :program:`colcon` mixins
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [colcon-mixins-start]
   :end-before: [colcon-mixins-end]


Clones the :program:`colcon` mixin repository
and adds default mixins for :program:`colcon`,
updating them as necessary.

Again, the directory configured for the :samp:`colcon-artefacts` plug is used.


Install :program:`rosdep` dependencies
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. literalinclude:: design-sdk/setup-base
   :language: shell
   :start-after: [rosdep-dependencies-start]
   :end-before: [rosdep-dependencies-end]


Initialises :program:`rosdep`,
a tool for installing system dependencies,
updates it for our ROS 2 distribution,
then installs dependencies for the project located under :file:`/project/`,
if any.

.. important::

   This means that the intended way of using this SDK
   is to put the ROS 2 sources in the *project directory* of the workshop
   *before* actually launching or refreshing the workshop;
   the sources will be mapped from the host to the :file:`/project/` directory,
   so this part of the hook will find and install the dependencies for the user.


Run-time behaviour
------------------

At run-time, SDK revisions are available inside the workshop;
under :file:`/var/lib/workshop/sdk/ros2/`,
you can see all SDK content that was packed, published and installed.
There, :file:`current/` always maps to the latest installed revision:

.. @artefact workshop shell
.. @artefact SDK revision

.. code-block:: console

   $ workshop shell ros2-jazzy
   workshop@ros2-jazzy-8584e57d$ ls /var/lib/workshop/sdk/ros2/current

   meta  sdk


The :file:`meta/` directory contains the definition,
whereas :file:`sdk/` stores hooks (and will store any other content
when respective features are eventually added).


Conclusion
----------

This guide presents one way to approach SDK design;
there are other options involving more hooks,
the :samp:`parts` mechanism and so on.
Let's go through the trade-offs here:

- One benefit is that the SDK itself is very lightweight

- Another advantage is its using the most up-to-date versions of all content

- The drawback is the content being downloaded and installed
  at every launch or refresh


In general, it's a perfectly viable approach
that you safely can adopt for your first SDK design.


See also
--------

Explanation:

- :ref:`exp_interfaces`
- :ref:`exp_sdk_definition`
- :ref:`exp_sdk_hooks`
- :ref:`exp_sdk_parts`

Reference:

- :ref:`ref_mount_interface`
- :ref:`ref_gpu_interface`
- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_hooks`
- :ref:`ref_sdk_parts`
