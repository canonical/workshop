.. meta::
   :description: Workshop explanation documentation, providing access to
                 explanations of core workshop concepts, change tracking,
                 and project management.

Workshops
=========

The eponymous *workshop* is the core idea behind |ws_markup|.


Core concepts
-------------

A workshop is a container-based development environment
defined in YAML and hosted by LXD.
Projects are the directories that hold workshop definitions
and mount inside the running containers:

.. toctree::
   :maxdepth: 1

   concepts
   projects
   multi-workshop-patterns


Operations and tooling
----------------------

|ws_markup| tracks state through a system of changes and tasks:

.. toctree::
   :maxdepth: 1

   changes-tasks
