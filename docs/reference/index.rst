.. _ref_index:

Reference
=========

These reference guides
provide technical background
that may be required to use |ws_markup| and |sdk_markup|.


Command-line interfaces
-----------------------

These articles share the usage details of the command-line tools
provided by |ws_markup| and |sdk_markup|:

.. toctree::
   :maxdepth: 2

   CLI <cli/index>


Definition formats
------------------

.. @artefact SDK
.. @artefact workshop (container)

Workshops and SDKs are defined in YAML and share a number of basic elements
such as plugs, base images and so on.
However, both definition types have different purposes and structure:

.. toctree::
   :maxdepth: 2

   Definitions <definitions/index>


Structure and behaviour
-----------------------

These topics provide detailed guidance on various aspects
of operating an SDK or a workshop at run-time.
Workshops are essentially made of SDKs;
to understand how a workshop runs and the status it has,
it's crucial to know how SDKs are structured and operated:

.. toctree::
   :maxdepth: 1

   sdks
   workshop-status
