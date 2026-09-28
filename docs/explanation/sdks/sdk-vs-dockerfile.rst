.. _exp_dockerfile_vs_sdk:

.. meta::
   :description: A comparison between traditional Dockerfiles and Workshop SDKs,
                 explaining how Workshop's approach to development environments
                 offers improved repeatability, persistence, and layering
                 compared to Docker-based solutions.

How SDKs compare to Dockerfiles
===============================

.. @artefact SDK
.. @artefact workshop (container)

|ws_markup| didn't occur in a vacuum;
there have been many attempts to provide developers with robust environments.
A common approach is to use Docker
to achieve repeatability, persistence, layering, and various other benefits
that the technology offers.

We won't dwell on the pros and cons here;
instead, let's discuss how a typical Dockerfile development environment
maps to a workshop and its SDKs.

.. note::

   We assume you're familiar with |sdk_markup| basics
   covered in the :ref:`tut_craft_sdks` tutorial section
   and have an understanding of Docker.


Feature discussion
------------------

To begin with, it's perfectly reasonable to draw a few comparisons
between Docker and the combination of |ws_markup| and |sdk_markup|.


:spellexception:`(Im)mutability`
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The first contrast comes from the overall approach:
Docker images are conceived to be immutable,
whereas workshops are designed to evolve over time.
This affects all aspects of their design and implementation,
including how Dockerfiles and SDKs are laid out, respectively.


Bind mounts and volumes
~~~~~~~~~~~~~~~~~~~~~~~

Docker provides several ways to manage data persistence and storage
such as the :samp:`VOLUME` instructions,
the :command:`docker volume` command
or the :option:`!--mount` and :option:`!-v` options in :command:`docker run`.
The expectations for their configuration are set by the image author
but the actual parameters are provided by users at the author's guidance;
the resulting manual process is error-prone and adds unnecessary overhead.

|ws_markup| and |sdk_markup| reciprocate this
with :ref:`mount interface <exp_mount_interface>` plugs
that are akin to Docker volumes
and the :command:`workshop remount` command
that enables remounting existing plugs to a given location.
However, the user can't create arbitrary mounts;
the choice is limited to what the SDKs offer.

In turn, this implies that the mount logic
in |ws_markup| and |sdk_markup|
is built into the SDK by its author,
not implemented manually by the user;
unless the user decides to intervene,
the mounts are managed automatically and largely stay hidden.


Resource usage
~~~~~~~~~~~~~~

For largely historical reasons,
the Docker way of accessing various host resources
can be notably inconsistent;
for example, enabling GPU pass-through is visibly different from SSH forwarding.

In contrast, |ws_markup| and |sdk_markup| unify these mechanisms
under the single concept of an :ref:`interface <exp_interface_concepts>`,
providing a consistent way to uniformly manage host resource access.


Parts and layers
~~~~~~~~~~~~~~~~

Docker relies on a temporally layered approach,
where each change is built on top of the previous one.

.. @artefact SDK hook

Our SDKs are structured using :ref:`parts <exp_sdk_parts>`;
their expressiveness makes them more diverse and semantically rich,
allowing the layout of an SDK to be formalized in a modular way.
If necessary, the layered approach
can be mimicked using :ref:`SDK hooks <exp_sdk_hooks>`.
|ws_markup| uses ZFS snapshots and clones
to cache the results of each :samp:`setup-base` hook.


Build commands
~~~~~~~~~~~~~~

In Docker,
build commands are typically bundled as :samp:`RUN` instructions.

In |sdk_markup| SDKs,
the :samp:`setup-base` :ref:`hook <exp_sdk_hooks>`
is responsible for building the workshop,
but other hooks add extra functionality with runtime events and health checks.

A related Docker pattern applies :samp:`USER`
to switch from root to a nonroot user,
followed by :samp:`RUN` instructions for user-level setup:

.. code-block:: docker

   # System setup as root (≈ setup-base)
   RUN apt-get update && apt-get install -y ...

   # Switch to non-root user and set up the project (≈ setup-project)
   USER appuser
   WORKDIR /home/appuser
   RUN pip install --user ...

In |ws_markup|,
this maps to the :samp:`setup-project` :ref:`hook <exp_sdk_hooks>`,
which inherently runs as the :samp:`workshop` user.
Unlike :samp:`setup-base`,
:samp:`setup-project` runs after interfaces are connected
and the :file:`/project/` directory is mounted,
so it can leverage available hardware
and install project-specific dependencies.


.. _exp_content_sharing:

Data persistence and sharing
-----------------------------

Consider this Docker command:

.. code-block:: console

   $ docker run --name share-example --entrypoint bash -it \
     -v ~/docker/kit/cache/Kit:/kit/cache:rw \
     -v ~/docker/cache/ov:/root/.cache/ov:rw \
     ...


All too familiar, isn't it?
When running a sufficiently complex container,
you need to mount a lot of directories to make it work,
and the handling of these mounts both inside and outside the container
can quickly become an overhead.

.. @artefact SDK

|ws_markup| addresses this issue by providing a way
to reuse and share content between the host and the workshop via SDKs
while keeping manual intervention to a necessary minimum.
Typically, workshops are isolated from each other and from the host system;
all data exchange is via the mount interface.

To use this interface, your SDK defines a
:ref:`mount interface plug <exp_mount_plug>`.
When a workshop uses the SDK,
an auto-assigned, noncustomizable source directory on the host
is mounted to the plug-defined target directory inside the workshop.
What's more, its contents are preserved during refresh operations.
In this way, |ws_markup| enables SDK data persistence and reuse
*inside* individual workshops.

Note, however,
that files created in the plug's target location by any means
will only be accessible to the workshop
to which that specific auto-assigned source directory is mounted to.
Other workshops, even if they use the same SDK,
cannot access these files and will not share them;
their source directories will be different.


Persistence and reuse between workshops
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

This is the simplest scenario;
you use the :samp:`mount` interface
to define the target directory
where the content will be mounted inside the workshop
per each directory you want to retain during the workshop's lifecycle.

.. @artefact sdkcraft (CLI)

.. code-block:: yaml
   :caption: sdkcraft.yaml

   name: data-science
   title: Data science SDK
   base: ubuntu@22.04
   summary: This SDK does some data science.
   description: |
     Besides doing actual data science,
     this SDK demonstrates content sharing and persistence between workshops
     by enabling two plugs that can store reusable data specific to the SDK.

   plugs:
     share-cache:
       interface: mount
       workshop-target: /opt/cache

     training-data:
       interface: mount
       workshop-target: /opt/training
       read-only: true


This SDK defines two mount plugs;
for each,
|ws_markup| creates a source directory on the host at runtime.
Both :samp:`workshop-target` directories inside the workshop
can be used by the SDK-specific logic
implemented via :ref:`hooks <exp_sdk_hooks>` and other features.

Additionally, you can mark a directory as `read-only`.
|ws_markup| will then enforce the immutability of resources in this directory
when they are accessed from inside the workshop.

Here's a corresponding workshop definition:

.. code-block:: yaml
   :caption: .workshop/data.yaml

   name: data
   base: ubuntu@22.04
   sdks:
     - name: data-science


The default host location
that |ws_markup| mounts to the target
is predefined as follows:

.. code-block:: none

   $XDG_DATA_HOME/workshop/id/<PROJECT ID>/<WORKSHOP>/mount/<SDK>/<PLUG>/


In the above example,
this would be
:file:`~/.local/share/workshop/id/<PROJECT ID>/<WORKSHOP>/mount/data-science/share-cache/`.
In particular,
this means that the SDK's plug in each workshop
will have its own unique source directory.


Share custom host content with a workshop
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

One issue that the previous scenario doesn't address
is customizing the source directory of a plug.
The :command:`docker run` example at the beginning illustrates this approach;
it explicitly lists the host directories to be mounted to each target.

This can also be done with |ws_markup|,
and the :command:`workshop remount` command is the key to it:

.. @artefact workshop remount

.. code-block:: console

   $ workshop remount data/data-science:share-cache ~/.local/cache/


This mounts a specific source location on the host, :file:`~/.local/cache/`,
to the target directory of the :samp:`share-cache` mount interface plug
under the :samp:`data-science` SDK in the :samp:`data` workshop defined above.


Feature mapping
---------------

.. @artefact SDK publisher

Any attempt at a straightforward comparison of these different,
albeit vaguely similar, technologies is mostly futile.
Again, a key difference is that a Dockerfile is controlled by the user,
but a workshop is *managed* by the user, yet it relies on publisher-defined SDKs
whose layout is beyond the user's reach.

This means that some capabilities of Docker
won't be available to a user of |ws_markup| alone,
so the functionality is split between the user-oriented |ws_markup|
and the publisher-focused |sdk_markup|.

Important Dockerfile instructions are mapped to |sdk_markup| as follows:

.. @artefact check-health
.. @artefact SDK definition


.. list-table::
   :header-rows: 1

   * - Dockerfile
     - SDKcraft

   * - :samp:`ADD`
     - :ref:`parts <exp_sdk_parts>`,
       :ref:`mount interface <exp_mount_interface>`

   * - :samp:`CMD`
     - :samp:`setup-base` :ref:`hook <exp_sdk_hooks>`

   * - :samp:`COPY`
     - :samp:`setup-base` :ref:`hook <exp_sdk_hooks>`

   * - :samp:`ENTRYPOINT`
     - :samp:`setup-base` :ref:`hook <exp_sdk_hooks>`

   * - :samp:`FROM`
     - :samp:`base` in the :ref:`SDK definition <exp_sdk_definition>`

   * - :samp:`HEALTHCHECK`
     - :samp:`check-health` hook

   * - :samp:`ONBUILD`
     - :samp:`setup-base` :ref:`hook <exp_sdk_hooks>`

   * - :samp:`RUN`
     - :samp:`setup-base`,
       :samp:`setup-project` :ref:`hooks <exp_sdk_hooks>`

   * - :samp:`USER`
     - :samp:`setup-project` :ref:`hook <exp_sdk_hooks>`

   * - :samp:`VOLUME`
     - :ref:`mount interface <exp_mount_interface>`


In turn, the CLI subcommands can be mapped like this:

.. list-table::
   :header-rows: 1

   * - Docker CLI
     - Workshop/SDKcraft CLI

   * - :command:`docker build`
     - :command:`sdkcraft build`, :command:`sdkcraft pack`

   * - :command:`docker exec`
     - :command:`workshop exec`, :command:`workshop shell`, :command:`workshop run`

   * - :command:`docker images`, :command:`docker ps`
     - :command:`workshop info`, :command:`workshop list`

   * - :command:`docker logs`
     - :command:`workshop changes`, :command:`workshop tasks`

   * - :command:`docker rm`, :command:`docker rmi`
     - :command:`workshop remove`

   * - :command:`docker run`
     - :command:`workshop launch`, :command:`workshop refresh`

   * - :command:`docker run --mount`, :command:`docker volume`
     - :command:`workshop remount`

   * - :command:`docker start`
     - :command:`workshop start`

   * - :command:`docker stop`
     - :command:`workshop stop`


.. _exp_ros2_case_study:

Case study: ROS 2
-----------------

For a specific example,
consider the
`Docker-based tutorial <https://docs.ros.org/en/jazzy/How-To-Guides/Setup-ROS-2-with-VSCode-and-Docker-Container.html>`__
for ROS 2,
the open-source robotics operating system.
The choice is influenced by many factors,
including the fact that we have a ROS 2 SDK available for comparison;
for details, refer to the corresponding how-to guide under `See also`_.

Nonetheless, we won't focus on the specifics of ROS 2 here;
instead, we discuss how certain parts
of an arbitrarily sophisticated Dockerfile
map to a similar SDK and the workshop that uses it.


Base image
~~~~~~~~~~

The example suggests using the :samp:`ros:rolling` tag for the
`Dockerfile <https://docs.ros.org/en/jazzy/How-To-Guides/Setup-ROS-2-with-VSCode-and-Docker-Container.html#edit-dockerfile>`_;
with a few `levels of indirection <https://hub.docker.com/_/ros/>`_,
it comes down to this (or similar) instruction:

.. code-block:: docker

   FROM ubuntu:noble


For |ws_markup| and |sdk_markup|,
this translates to :samp:`ubuntu@24.04`
in the :ref:`SDK definition <exp_sdk_definition>`
and the :ref:`workshop definition <ref_workshop_definition>`.


.. _exp_docker_project:

Project workspace
~~~~~~~~~~~~~~~~~

The
`project workspace
<https://docs.ros.org/en/jazzy/How-To-Guides/Setup-ROS-2-with-VSCode-and-Docker-Container.html#configure-workspace-in-docker-and-vs-code>`_
in the example is defined as a bind mount that eventually becomes this:

.. code-block:: console
 
   $ docker run -it \
     --mount type=bind,source=/home/user/ros-project,target=/home/ws/src,consistency=cached \
     # ...


Its counterpart in |ws_markup| is the *project directory*
where the workshop was defined and launched;
it is automatically mounted as :file:`/project/` when the workshop is started:

.. @artefact workshop launch

.. code-block:: console

   $ workshop launch ros2jazzy  # must be run in the project directory


No explicit configuration is needed;
this behavior is intentionally consistent across all workshops.


Bind mounts
~~~~~~~~~~~

The ROS 2 example defines a
`few more mounts
<https://docs.ros.org/en/jazzy/How-To-Guides/Setup-ROS-2-with-VSCode-and-Docker-Container.html#edit-devcontainer-json-for-your-environment>`_;
a complete :command:`docker run` command may look like this:

.. code-block:: console

   $ docker run -it \
     --name ros2_container \
     --mount type=bind,source=/home/user/ros-project,target=/home/ws/src,consistency=cached \
     --mount type=bind,source=/home/user/.ros,target=/root/.ros,consistency=cached \
     --mount type=bind,source=/tmp/.X11-unix,target=/tmp/.X11-unix,consistency=cached \
     --mount type=bind,source=/dev/dri,target=/dev/dri,consistency=cached \
     ros2


In |ws_markup| and |sdk_markup|,
additional filesystem mounts are defined by the SDK author or the user
using the :ref:`mount interface <exp_mount_interface>`:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   plugs:
     ros-cache:
       interface: mount
       workshop-target: /home/workshop/.ros
   # ...


Just like with the :ref:`project files <exp_docker_project>`,
this avoids the need for manual setup when starting the workshop:

.. code-block:: console

   $ workshop launch ros2jazzy  # the plugs are mounted automatically


Again,
|ws_markup| and |sdk_markup|
have no direct counterpart to bind mounts;
plugs are more similar to Docker volumes.
Yet, the :command:`workshop remount` command
enables remounting existing plugs to new host directories:

.. @artefact workshop remount

.. code-block:: console

   $ workshop remount ros2jazzy/ros2:ros-cache ~/new-cache-mount/


Thus,
|ws_markup| and |sdk_markup|
largely leave the design of mount points to the SDK author,
allowing the user to rely on their default, well-defined behavior
with the extra option of adjusting them if necessary.


Build commands
~~~~~~~~~~~~~~

Normally, a :samp:`RUN` instruction in a Dockerfile
translates to the :samp:`setup-base` and :samp:`setup-project`
:ref:`hooks <exp_sdk_hooks>` in an SDK pretty well.
Here, the steps to
`set up keys <https://github.com/osrf/docker_images/blob/7f98ddd88d872299c45b60c8bcd70d4eb6665222/ros/rolling/ubuntu/noble/ros-core/Dockerfile#L19>`_,
then `configure the repos <https://github.com/osrf/docker_images/blob/7f98ddd88d872299c45b60c8bcd70d4eb6665222/ros/rolling/ubuntu/noble/ros-core/Dockerfile#L29>`_
and `install the packages <https://github.com/osrf/docker_images/blob/7f98ddd88d872299c45b60c8bcd70d4eb6665222/ros/rolling/ubuntu/noble/ros-core/Dockerfile#L38>`_
largely stay the same.

However, :samp:`setup-project` runs with the project directory already mounted,
so any steps that rely on the contents of the project itself
can be implemented with the same hook.
In particular, this enables the ROS 2 SDK
to transparently identify and install project-specific dependencies.


See also
--------

Explanation:

- :ref:`exp_mount_interface`
- :ref:`exp_projects`
- :ref:`exp_sdks`


Reference:

- :ref:`ref_mount_interface`
- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_hooks`
