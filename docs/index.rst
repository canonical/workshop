:relatedlinks: [Diátaxis](https://diataxis.fr/)

.. _home:

Workspace
=========

**Workspace automates the configuring and management
of reproducible development environments**.

**Use a straightforward YAML to define your development environment**.
Workspace will create a system container,
install specified SDKs and packages,
and control its behaviour with life cycle hooks.
VS Code, Jupyter Lab and other IDEs can discover and use your workspace
as a work environment.
Dispose the environment when done and keep the host system clean.

**Make the knowledge of your project's dev environments explicit and shared**.
New contributors can start with a single command
that launches the required workspace.
It is easier to debug issues in any of the project's supported environments,
perform code reviews or experiment in a separate light-weight container.

It is common to have a non-trivial project setup
with dependencies on particular Linux distributions,
SDKs from multiple publishers,
and system and language packages.
Most such projects can organise setup complexity with Workspace.
Examples include AI/ML, Robotics, IoT, EdTech and similar domains.

---------


In this documentation
---------------------

.. grid:: 1 1 2 2

   .. grid-item:: :doc:`Tutorial <tutorial/index>`

      **Start here**: a hands-on introduction to Workspace for new users


   .. grid-item:: :doc:`Explanation <explanation/index>`

      **Discussion and clarification** of key topics

---------


.. toctree::
   :hidden:
   :maxdepth: 2

   tutorial/index
   explanation/index
   ReadMe <README>
