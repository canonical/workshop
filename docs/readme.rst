Workshop
========

.. image:: https://readthedocs.com/projects/canonical-workshop/badge/?version=latest&token=a8c81a46da98f75a366a1eef905457dadfa50c23cf3a1c1929a81af05ffea85d
   :target: https://canonical-workshop.readthedocs-hosted.com/latest/?badge=latest
   :alt: Documentation Status

**A tool for defining and handling ephemeral development environments**.


Getting Started
---------------

Follow the sections below,
or refer to the
`Tutorial
<https://canonical-workshop.readthedocs-hosted.com/latest/tutorial/>`_
in our docs for a more detailed introduction to Workshop.

To join the development effort, see `How to contribute <contributing.rst>`_.

To know more about `SDKcraft <https://github.com/canonical/sdkcraft/>`_,
the SDK authoring tool for Workshop,
see the
`how-to guide
<https://canonical-workshop.readthedocs-hosted.com/latest/how-to/use-sdkcraft/>`_
in our docs.

Installation
~~~~~~~~~~~~

Workshop requires
`LXD 6.3+ <https://canonical.com/lxd>`_
for low-level operation.

Check whether it's configured:

.. code-block:: console

   lxc info | grep 'server_version:'

     server_version: "6.3"

If the command displays an older version
or returns an error indicating LXD is missing,
install a recent LXD version with :program:`snap`:

.. code-block:: console

   sudo snap install lxd --channel=6/stable  # to install
   sudo snap refresh lxd --channel=6/stable  # to update


Next, download the latest Workshop snap from the
`Releases <https://github.com/canonical/workshop/releases/>`_
page on GitHub and install it,
using the options
`--dangerous <https://snapcraft.io/docs/install-modes>`_
and
`--classic <https://snapcraft.io/docs/install-modes>`_,
for example:

.. code-block:: console

   sudo snap install --dangerous --classic ./workshop_0.1.17_amd64.snap


Launching workshops
-------------------

In the directory of the project
that you want to use with Workshop,
create a workshop definition file named ``workshop.yaml``
to list your project's prerequisites,
then run ``workshop launch``:

.. code-block:: console

   cat > workshop.yaml <<EOF -
   name: golang
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: latest/stable
   EOF

   workshop launch


Workshop downloads and installs the SDKs your definition lists;
the project is now ready to use them.
