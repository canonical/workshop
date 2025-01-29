.. _exp_ssh_interface:

SSH interface
=============

The SSH interface
provides access to the host system's SSH agent
from inside the workshop,
allowing it to securely use the host's SSH keys and configuration.

By using the interface,
the SDK publisher allows the workshop to connect to the host's SSH agent,
which can be useful in various SDK-specific tasks
such as cloning private repositories, accessing remote machines and so on.

.. _exp_ssh_plug:

SSH interface plug
------------------

An essential element here is the SSH interface plug,
which is declared in the SDK definition.

Its structure includes just the name of the plug and the interface;
both must be set to :samp:`ssh-agent`.

Defining the plug in an SDK
allows the workshops using this SDK to connect to the host's SSH agent,
which can be useful in various SDK-specific tasks
such as cloning private repositories, accessing remote machines and so on.


.. _exp_ssh_slot:

SSH interface slot
------------------

To let SDKs in a workshop access the host's SSH agent,
|ws_markup| provides an SSH interface slot
that multiple SSH interface plugs can access.

When the SDK is installed at run-time during launch and refresh operations,
|ws_markup| checks that the plug targeting the slot
passes :ref:`validation <exp_interfaces_validation>`;
if it does,
it can be connected.


Connection
----------

The interface isn't connected automatically at launch and refresh
for security reasons.
The :command:`workshop connect` and :command:`workshop disconnect` commands
can be invoked manually after the workshop has started:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop connect ws/ssh-sdk:ssh-agent
   $ workshop disconnect ws/ssh-sdk:ssh-agent


Establishing a connection means
a proxy Unix domain socket has been created
and a corresponding :envvar:`$SSH_AUTH_SOCK` value
has been set for the :samp:`workshop` user,
so the host's SSH identities and configuration
are available inside the workshop.

To check if the interface is connected:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections --all

     Interface  Plug                  Slot                 Notes
     ...
     ssh-agent  ws/ssh-sdk:ssh-agent  ws/system:ssh-agent  manual


So the host's SSH identities and configuration
are available inside the workshop:

.. code-block:: console

   $ workshop shell ws
   workshop@ws-8584e571$ echo $SSH_AUTH_SOCK

     /var/lib/workshop/ws-ssh-agent.ssh

   workshop@ws-8584e571$ ssh-add -l

     4096 SHA256:cb19/bE/6irqhII1KbQqRmo1royWi58qcUD9MEn/9fE user@example.com (RSA)


See also
--------

Explanation:

- :ref:`exp_interfaces`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdk_definition`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_shell`
