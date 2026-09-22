.. meta::
   :description: Workshop interfaces documentation, providing
                 access to explanations of interface concepts and specific
                 interface types used for resource sharing between containers.

Interfaces
==========

Interfaces allow communication and resource sharing
between a workshop and the host system,
as well as between the different SDKs that are part of a workshop.


General concepts
----------------

Start here to understand how interfaces use plugs and slots
to connect SDKs to host resources and to each other:

.. toctree::
   :maxdepth: 1

   concepts
   plugs-and-slots


Hardware interfaces
-------------------

Hardware interfaces give workshops access to host hardware
such as displays, GPUs, and cameras:

.. toctree::
   :maxdepth: 1

   camera-interface
   custom-device-interface
   desktop-interface
   gpu-interface
   virtualization-interface


Data and connectivity
---------------------

Filesystem mounts, SSH agent forwarding, and network sharing
pass through this group of interfaces:

.. toctree::
   :maxdepth: 1

   mount-interface
   ssh-interface
   tunnel-interface
