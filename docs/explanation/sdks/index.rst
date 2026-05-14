.. meta::
   :description: Workshop SDK documentation, providing access to
                 explanations of SDK concepts, data storage, lifecycle hooks,
                 and comparisons with traditional container approaches.

SDKs
====

.. @artefact SDK

SDKs are the pre-built, reusable blocks of functionality
involved in the definition, design, distribution,
and day-to-day operation of a workshop.


Understanding SDKs
------------------

At their core, SDKs are bundles of software dependencies
distributed through the SDK Store or defined locally;
they can pre-package libraries, tools, and configurations
or install them directly into a workshop.

.. toctree::
   :maxdepth: 1

   concepts
   Runtime hooks <runtime-hooks>
   Parts <parts>


SDK design
----------

When you are creating SDKs, it helps to understand
how they compare to traditional container approaches
and what design patterns lead to maintainable, reusable packages:

.. toctree::
   :maxdepth: 1

   Design best practices <best-practices>
   SDKs versus Dockerfiles <sdk-vs-dockerfile>


Operations and tooling
----------------------

|ws_markup| exposes available SDKs through the :program:`sdk` CLI,
lets SDK authors build and publish them with the :program:`sdkcraft` CLI,
and ships the in-workshop :program:`workshopctl` helper
for SDK hooks to talk back to the daemon:

.. toctree::
   :maxdepth: 1

   sdk-cli
   sdkcraft-cli
   workshopctl-cli
