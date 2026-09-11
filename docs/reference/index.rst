.. _ref_index:

.. meta::
   :description: Workshop reference guides, providing technical background
                 for using Workshop and SDKcraft, including command-line tools.

Reference
=========

These reference guides
provide technical background
that may be required to use |ws_markup| and |sdk_markup|.


.. _ref_cli:

Command-line tools
------------------

|ws_markup| and |sdk_markup| ship with a small set of command-line tools:

.. toctree::
   :titlesonly:
   :class: flat-toctree

   CLI <cli/index>


.. _ref_definitions:

Definition file formats
-----------------------

.. @artefact SDK
.. @artefact workshop (container)

Workshops and SDKs are defined in YAML and share a number of basic elements
such as plugs, base images and so on.
However, both definition types have different purposes and structure:

.. toctree::
   :titlesonly:
   :class: flat-toctree

   Definition files <definition-files/index>


Structure and behavior
----------------------

Workshops are largely made of SDKs;
understanding how a workshop runs and the status it carries
starts with how SDKs are structured and operated at run-time:

.. toctree::
   :titlesonly:
   :class: flat-toctree

   sdks
   workshops
   workshop-status


.. _ref_ai:

AI agents
---------

|ws_markup| exposes documentation in LLM-readable form
and ships three agentic skills that wrap its CLIs
for coding agents:

.. toctree::
   :titlesonly:
   :class: flat-toctree

   ai-agents



Reference implementations
-------------------------

.. @artefact SDK

These real-life examples on GitHub,
maintained by the |ws_markup| team,
are meant to showcase different SDK patterns and workshop implementations.
Study them to better understand SDK design and workshop creation:

- https://github.com/canonical/reference-sdks
- https://github.com/canonical/reference-workshops
