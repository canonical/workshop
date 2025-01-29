.. _exp_index:

Explanation
===========

These explanatory articles cover the main building blocks of |ws_markup|.
To start using |ws_markup| and |sdk_markup|,
it is important to understand how these concepts fit together.


Workshops and projects
----------------------

.. @artefact project
.. @artefact workshop (container)
.. @artefact workshop definition

A *workshop* is a container that enables consistent environment builds.
It is tied to a definition that lists SDKs and is stored as a :file:`.yaml` file
under a project directory.
A *project* is the working directory where workshop definitions are placed.
When you start a workshop, the project directory is mounted inside it,
so storing repositories, code, or data such as models in the project directory
enables you to use them inside the workshop.

.. toctree::
   :maxdepth: 2
   :titlesonly:

   workshops/index


SDKs
----

.. @artefact SDK

With |sdk_markup|, you can package and publish software dependencies
as isolated *SDKs* to be used in a workshop definition by |ws_markup|,
instead of managing them system-wide or through container images.
SDKs encapsulate all required functionality,
keeping installations clean and limiting access to system-level capabilities.
Publishers handle installation and updates for SDKs,
freeing users from maintaining complex image definitions or configurations.

.. toctree::
   :maxdepth: 2
   :titlesonly:

   sdks/index


Interfaces
----------

Interface connections are a mechanism for communication and resource sharing.
It is an integral part of workshop confinement,
ensuring that each workshop operates in its own isolated environment,
while still allowing controlled interactions among the SDKs and with the host.

.. toctree::
   :maxdepth: 2
   :titlesonly:

   interfaces/index


Security considerations
-----------------------

This overview discusses the security aspects of |ws_markup| and |sdk_markup|,
such as isolation, privileges, relevant risks and interface mechanics.

.. toctree::
   :maxdepth: 1

   Security <../security>
