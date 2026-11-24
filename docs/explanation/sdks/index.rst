.. meta::
   :description: Workshop SDK documentation, providing access to
                 explanations of SDK concepts, data storage, runtime hooks,
                 and comparisons with traditional container approaches.

SDKs
====

SDKs are the pre-built, reusable blocks of functionality
involved in the definition, design, distribution,
and day-to-day operation of a workshop.


Understanding SDKs
------------------

At their core, SDKs are bundles of software dependencies
distributed through the SDK Store or defined locally;
they can pre-package libraries, tools, and configurations
or install them directly into a workshop.

.. toctree::
   :maxdepth: 1

   concepts
   Parts <parts>
   Runtime hooks <runtime-hooks>


SDK lifecycle
-------------

Most SDKs move through the same stages,
from a quick local hack in a single workshop
to a consumable artifact on the SDK Store:

.. toctree::
   :maxdepth: 1

   lifecycle


SDK design
----------

When you are creating SDKs, it helps to understand
how they compare to traditional container approaches
and what design patterns lead to maintainable, reusable packages:

.. toctree::
   :maxdepth: 1

   Design best practices <best-practices>
   SDKs versus Dockerfiles <sdk-vs-dockerfile>
