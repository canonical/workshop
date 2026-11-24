.. _exp_index:

.. meta::
   :description: An overview of Workshop's core concepts, explaining how
                 workshops, projects, and SDKs work together to create
                 consistent development environments and help build workflows.

Explanation
===========

These explanatory articles cover the main building blocks of |ws_markup|.
To start using |ws_markup| and |sdk_markup|,
it is important to understand how these concepts fit together.


.. _exp_arch:

Architecture
------------

|ws_markup|'s architecture combines a set of system components
with well-defined runtime behavior
to provide container-isolated development environments.

.. toctree::
   :titlesonly:
   :class: flat-toctree

   architecture/index


.. _exp_workshop:

Workshops and projects
----------------------

Workshops are development environments, each running in a container,
mapping your project to its contained dependencies.
In turn, a project is a working directory
where multiple workshop definitions can be placed.

.. toctree::
   :titlesonly:
   :class: flat-toctree

   workshops/index


.. _exp_sdks:

SDKs
----

SDKs are packages of software dependencies that can be installed in workshops
to create tailored development environments.

.. toctree::
   :titlesonly:
   :class: flat-toctree

   sdks/index


.. _exp_interfaces:

Interfaces
----------

Interfaces allow communication and resource sharing
between a workshop and the host system,
as well as between the different SDKs that are part of a workshop.

.. toctree::
   :titlesonly:
   :class: flat-toctree

   interfaces/index


Command-line tools
------------------

|ws_markup| works through four command-line tools,
one for each audience and stage of the workflow.

.. toctree::
   :titlesonly:
   :class: flat-toctree

   cli


Security considerations
-----------------------

This overview discusses the security aspects of |ws_markup| and |sdk_markup|,
such as isolation, privileges, relevant risks, and interface mechanics.
See :doc:`Security policy </security>`.
