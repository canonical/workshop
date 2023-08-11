:relatedlinks: [Diátaxis](https://diataxis.fr/)

.. _home:

Workspace
=========

**Workspace automates configuration and management
with reproducible development environments**.

**Define your dev environment in straightforward YAML**.
The tool consumes it to create a contained workspace,
installs the SDKs and packages it lists,
and adds life cycle hooks for run-time control.
IDEs such as Visual Studio Code or Jupyter Lab
can discover workspaces and leverage them in their operation;
when you're done with a workspace,
its disposal doesn't affect the host system.

**Untangle the know-how that was weaved into your project**.
An environment that could take hours of setup
can be launched with one command;
workspaces enhance issue reproduction across platforms,
facilitate collaboration in code reviews,
and confine hackish experiments in lightweight containers.

**Mitigate your setup's complexity with Workspace.**
AI/ML, robotics, IoT, EdTech, and similar domains
commonly have less-than-trivial project layouts
that depend on multiple Linux distributions,
a plethora of SDKs from different publishers,
and a grocery list of libraries and programming languages.

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
