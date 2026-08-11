.. _how_jetbrains_gateway:

.. meta::
    :description: How-to guide on running JetBrains Gateway with Workshop
                  for remote development.

How to use JetBrains Gateway with Workshop
==========================================

.. @tests not applicable: requires JetBrains Gateway client and SSH key flow

`JetBrains Gateway
<https://www.jetbrains.com/remote-development/gateway/>`_
allows you to connect to remote development environments
while using your favorite JetBrains IDEs (IntelliJ IDEA, PyCharm, and so on).
A workshop can serve as the remote development target
for JetBrains Gateway,
letting you use your favorite JetBrains IDEs against |ws_markup|.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- `JetBrains Gateway <https://www.jetbrains.com/remote-development/gateway/>`__
  installed on your host system.


SSH access
----------

|ws_markup| sets up SSH access to workshops on its own.
It configures the OpenSSH client on the host
and maintains a per-user certificate authority
that signs a host certificate for every workshop
and a user certificate for the :samp:`workshop` user.
Connecting to a workshop by its hostname
needs no key management, passwords, or host-key prompts.


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
when configuring JetBrains Gateway.


Connect with Gateway
--------------------

#. Open JetBrains Gateway.

#. Create a new SSH connection using these values:

   - :guilabel:`Username`: :samp:`workshop`

   - :guilabel:`Host`:
     The hostname from :command:`workshop info`,
     for example :samp:`dev.my-project.wp`

   - :guilabel:`Port`: :samp:`22`


#. Click :guilabel:`Check connection and Continue`.

#. At :guilabel:`Choose IDE and Project`,
   select your preferred JetBrains :guilabel:`IDE version`, e.g. PyCharm.

   For :guilabel:`Project directory`, enter :file:`/project/`.

#. Click :guilabel:`Download IDE and Connect`,
   then wait for the IDE to install and launch;
   this may take a few minutes.
   After the IDE starts,
   log in and proceed as usual.


Your JetBrains IDE will now run remotely in the workshop
while providing a native desktop experience.


See also
--------

Explanation:

- :ref:`exp_arch_network`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_workshop_info`
- :ref:`ref_workshop_launch`
