.. _exp_content_sharing:

Data persistence and sharing
============================

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
an auto-assigned, non-customisable source directory on the host
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
---------------------------------------

This is the simplest scenario;
you use the :samp:`mount` interface
to define the target directory
where the content will be mounted inside the workshop
per each directory you want to retain during the workshop's life cycle.

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
|ws_markup| creates a source directory on the host at run-time.
Both :samp:`workshop-target` directories inside the workshop
can be used by the SDK-specific logic
implemented via :ref:`hooks <exp_sdk_hooks>` and other features.

Additionally, you can mark a directory as `read-only`.
|ws_markup| will then enforce the immutability of resources in this directory
when they are accessed from inside the workshop.

Here's a corresponding workshop definition:

.. code-block:: yaml
   :caption: .workshop.data.yaml

   name: data
   base: ubuntu@22.04
   sdks:
     - name: data-science
       channel: latest/stable


The default host location
that |ws_markup| mounts to the target
is pre-defined as follows:

.. code-block:: none

   $XDG_DATA_HOME/workshop/project/<PROJECT ID>/mount/<WORKSHOP>_<SDK>_<PLUG>.sdk/


In the above example,
this would be
:file:`~/.local/share/workshop/project/<PROJECT ID>/mount/data_data-science_share-cache.sdk/`.
In particular,
this means that the SDK's plug in each workshop
will have its own unique source directory.


Share custom host content with a workshop
-----------------------------------------

One issue that the previous scenario doesn't address
is customising the source directory of a plug.
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


See also
--------

Explanation:

- :ref:`exp_mount_interface`
- :ref:`exp_sdk`


Reference:

- :ref:`ref_mount_interface`
