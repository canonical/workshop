.. _how_vscode_connect_remote:

.. meta::
   :description: How-to guide on connecting a local Visual Studio Code
                 instance to a remote workshop.

How to connect your local VS Code to a workshop
===============================================

.. @tests in tests/docs-how-to/connect-vscode/task.yaml

.. @artefact workshop (container)

A local VS Code instance can connect to a remote workshop environment
via the :samp:`vscode-remote` SDK,
giving you the full VS Code experience against |ws_markup|.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- The `VS Code Remote Development extension pack <https://code.visualstudio.com/docs/remote/remote-overview>`__
  installed in VS Code.


Configure SSH access
--------------------

On most distributions,
|ws_markup| configures OpenSSH automatically.
To check the configuration, run:

.. code-block:: console

   $ ssh -G workshop.wp | grep -i '^User\>'

   user workshop


If it displays a different user from :samp:`workshop`,
then OpenSSH is not configured correctly.
To fix it, add this line to your host user's
:file:`~/.ssh/config` file:

.. code-block:: text
   :caption: ~/.ssh/config

   Include /var/snap/workshop/common/workshop/ssh/config


Add the SDK (optional)
----------------------

Add the :samp:`vscode-remote` SDK to your workshop definition:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 4

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: vscode-remote


This preserves the files installed by VS Code
when you use :command:`workshop refresh`.


Launch the workshop
-------------------

Launch the workshop if it isn't already running:

.. code-block:: console

   $ workshop launch

Then find the workshop hostname:

.. code-block:: console

   $ workshop info

   name:      dev
   base:      ubuntu@24.04
   project:   ~/my-project
   hostname:  dev.my-project.wp
   ...


Use the hostname from the :command:`workshop info` output
when configuring VS Code.


Connect with VS Code
--------------------

In VS Code, press :guilabel:`F1` to open the command palette,
start typing :guilabel:`Connect to Host`,
then choose the :guilabel:`Remote-SSH` option.
Enter the hostname from :command:`workshop info`,
e.g. :samp:`dev.my-project.wp`.
In the terminal prompt, you'll see that the IDE is running inside your workshop.

.. note::

   If you're having trouble finding the :guilabel:`Remote-SSH` option,
   mind that it's enabled by the :samp:`Remote - SSH` extension
   from the extension pack mentioned above.


See also
--------

Explanation:

- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_workshop_info`
- :ref:`ref_workshop_launch`
