Workshop
========
|coverage badge| |docs badge|

.. |coverage badge| raw:: html
   <a href=https://github.com/canonical/workshop/wiki/coverage/coverage.html>
   <img src="https://github.com/canonical/workshop/wiki/coverage/coverage.svg" style="text-decoration: none;">

.. |docs badge| raw:: html
   <a href=https://canonical-workshop.readthedocs-hosted.com/en/latest/?badge=latest>
   <img src="https://readthedocs.com/projects/canonical-workshop/badge/?version=latest&token=a8c81a46da98f75a366a1eef905457dadfa50c23cf3a1c1929a81af05ffea85d" style="text-decoration: none;">

**A tool for defining and handling ephemeral development environments**.


Getting Started
---------------

Follow the sections below
or refer to the
`Tutorial
<https://canonical-workshop.readthedocs-hosted.com/en/latest/tutorial/>`_
in our docs for a more detailed introduction to Workshop.

To join the development effort, see `How to contribute <contributing.rst>`_.

To know more about `SDKcraft <https://github.com/canonical/sdkcraft/>`_,
the SDK authoring tool for Workshop,
see the
`how-to guide
<https://canonical-workshop.readthedocs-hosted.com/en/latest/how-to/use-sdkcraft/>`_
in our docs.

Installation
~~~~~~~~~~~~

Workshop requires
`LXD 5.21+ <https://canonical.com/lxd>`_
for low-level operation:

.. code-block:: console

   sudo snap install lxd
   sudo lxd init --auto


Download the latest snap from the
`Releases <https://github.com/canonical/workshop/releases/>`_
page on GitHub and install it,
using the options
`--dangerous <https://snapcraft.io/docs/install-modes>`_
and
`--classic <https://snapcraft.io/docs/install-modes>`_,
for example:

.. code-block:: console

   sudo snap install --dangerous --classic ./workshop_0.1.5_amd64.snap


Launching workshops
-------------------

In the ``.workshop`` directory of the project
that you want to use with Workshop,
create a workshop definition file named ``workshop.yaml``
to list your project's prerequisites,
then run ``workshop launch``:

.. code-block:: console

   mkdir -p .workshop

   cat > workshop.yaml <<EOF -
   name: golang
   base: ubuntu@22.04
   sdks:
     go:
       channel: latest/stable
   EOF

   workshop launch


Workshop downloads and installs the SDKs your definition lists;
the project is now ready to use them.
