.. _how_troubleshoot:

.. meta::
   :description: How-to guide on troubleshooting Workshop issues, including verifying
                 installation, updating prerequisites, and checking versions.

How to troubleshoot |ws_markup|
===============================

If you notice issues with workshops, projects, or |ws_markup| in general,
it may be time to verify or update the installation or prerequisites.


Check and update version
------------------------

Ensure your version matches the latest
`release <Releases_>`_ on GitHub:

.. code-block:: console

   $ snap info workshop

     ...
     installed:    0.1.13 (x18) 24MB classic


If it's outdated, download and install the update:

.. code-block:: console

   $ sudo snap install --dangerous --classic ./workshop_0.1.22_amd64.snap


Install and start LXD
---------------------

A major prerequisite for |ws_markup| is `LXD`_;
ensure it's installed and running:

.. code-block:: console

   $ sudo snap install lxd
   $ sudo snap start --enable lxd.daemon
   $ sudo snap services lxd


You may need to add yourself to the :samp:`lxd` group to access its resources:

.. code-block:: console

   $ sudo usermod -a -G lxd $USER

As a final step, see the
`troubleshooting guides <https://documentation.ubuntu.com/lxd/latest/howto/troubleshoot/>`_
in LXD documentation.


Check the snap logs
-------------------

Before resorting to the :ref:`debugging guide <how_debug_issues_workshops>`
for individual workshops, review the snap's logs:

.. code-block:: console

   $ sudo snap logs workshop


.. _how_troubleshoot_lxc:

Explore LXD containers
----------------------

.. @artefact workshop (container)

If you notice an issue with a specific workshop,
use the `LXC`_ utility to identify and troubleshoot it.

For instance, if you've deleted a project
without first removing the associated workshops,
you can list all LXD projects to locate the orphaned containers.
These will appear under :samp:`workshop.<USER>`
and include the |ws_markup| project ID in their names:

.. code-block:: console

   $ sudo lxc list --all-projects

     ...
     | workshop.user | nimble-ec275767 | STOPPED | | | CONTAINER | 0 |


Next, you can manually delete a container:

.. code-block:: console

   $ sudo lxc delete nimble-ec275767 --project workshop.user


Or, you can shell into the container to recover its data:

.. code-block:: console

   $ sudo lxc shell nimble-ec275767 --project workshop.user


If the container fails to start,
use :command:`lxc info` to view the latest log entries:

.. code-block:: console

   $ sudo lxc info --show-log nimble-ec275767 --project workshop.user


To increase log verbosity,
you can reconfigure LXD with :program:`snap`:

.. code-block:: console

   $ sudo snap set lxd daemon.debug=true
   $ sudo snap restart lxd.daemon


Use other relevant `LXC`_ commands to continue your investigation.


See also
--------

How-to guides:

- :ref:`how_debug_issues_workshops`
