.. _exp_secret_interface:

.. meta::
   :description: Explanation of the secret interface that lets SDKs in a workshop
                 request a credential from the host keyring on demand,
                 covering the plug and slot roles, explicit connection,
                 how a value reaches a command or a systemd service,
                 request records, and what a connection exposes.

Secret interface
================

.. @artefact secret interface

The secret interface lets an SDK use a credential,
such as an API key,
that stays in the user's host keyring.
Neither the SDK definition nor the workshop definition
needs to contain the value,
and neither does anything else that ships with the SDK or the project.

The interface splits the job between the two people involved.
The SDK author knows that the SDK needs a credential,
so the SDK declares a :samp:`secret` plug.
Only the user knows where their credential is kept,
so the workshop definition adds a :samp:`secret` slot
that tells |ws_markup| how to find it in the host keyring.
Connecting the plug to the slot lets processes in the workshop
request the value whenever they need it.


.. _exp_secret_plug:

Secret interface plug
---------------------

A :samp:`secret` plug declares that the SDK needs one credential.
Only regular SDKs can declare it;
the :ref:`system SDK <exp_system_sdk>` can't.
The plug takes any valid plug name
and carries no attributes,
so it describes the need, not the value or where to find it:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   plugs:
     api-key:
       interface: secret


The plug's name becomes part of how the SDK asks for the value.
Inside the workshop,
the secret is identified as :samp:`{SDK}.{PLUG}`,
which is :samp:`secret-demo.api-key`
for the plug above in an SDK named :samp:`secret-demo`.
Both the SDK's commands and its systemd units request the value by that identifier.
Users connect the plug by its name too,
so renaming it in a later SDK version
drops their connection at the next refresh.


.. _exp_secret_slot:

Secret interface slot
---------------------

A :samp:`secret` slot describes one item in the host keyring.
Only the system SDK can have secret slots,
because a slot holds the host keyring lookup details for one workshop.
Users add them under the :samp:`system` SDK in the workshop definition:

.. code-block:: yaml
   :caption: workshop.yaml

   sdks:
     - name: system
       slots:
         demo-key:
           interface: secret
           attributes:
             service: secret-demo
             account: test
     - name: secret-demo


The slot carries lookup details only, never the value.
Its :samp:`attributes` are the name and value pairs
that the keyring item was stored with;
a lookup succeeds only when exactly one item carries all of them.
An optional :samp:`collection` selects the keyring collection to search,
and the collection that the keyring's :samp:`default` alias points to
is used when the slot doesn't name one.
A non-default value is matched against collection labels
and must identify exactly one collection.
For the full grammar,
see :ref:`ref_workshop_definition_interfaces`.

When the workshop is launched or refreshed,
|ws_markup| :ref:`validates <exp_interfaces_validation>`
the slot's keys and their types,
but it doesn't look in the keyring.
Whether a matching item exists
is only known when a process requests the value.


Connection
----------

|ws_markup| never connects a :samp:`secret` plug on its own:
a secret reaches a workshop only by an explicit decision of the user.
The interface policy denies auto-connection outright,
so listing the pair in the workshop definition's :samp:`connections`
doesn't connect it either;
see :ref:`exp_interface_auto_connection`.
Users connect and disconnect the plug
with :command:`workshop connect` and :command:`workshop disconnect`:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop connect dev/secret-demo:api-key dev/system:demo-key
   $ workshop disconnect dev/secret-demo:api-key


The connection is listed as manual:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections dev

     INTERFACE  PLUG                     SLOT                 NOTES
     ...
     secret     dev/secret-demo:api-key  dev/system:demo-key  manual


Connecting reads nothing from the keyring
and puts nothing in the workshop;
it only permits later requests.

For an LXD container workshop,
:command:`workshop refresh` keeps the connection,
including when the refresh changes the slot's attributes,
so a corrected lookup takes effect without reconnecting.
VM workshops don't restore interface connections during a refresh;
the user must reconnect the plug afterward.
A :command:`workshop restore` drops it, like any manual connection,
and so does a refresh that removes the plug from the SDK
or the slot from the workshop definition;
see :ref:`exp_workshop_connection_lifecycle`.


.. _exp_secret_delivery:

How a value reaches the workshop
--------------------------------

Nothing is fetched in advance.
|ws_markup| looks the value up in the host keyring
each time a process in the workshop requests it.
A direct command request writes the value to standard output;
a systemd request makes it available as a credential file
to the unit's user and root.
|ws_markup| doesn't persist the value;
it handles the value only transiently while fulfilling the request.

Because every request is a fresh lookup,
the keyring stays in control:
a value changed in the keyring takes effect on the next request,
and a request made while the keyring is locked fails.
|ws_markup| looks the item up through the D-Bus session bus
of the user's desktop session,
where a keyring service implementing the freedesktop.org Secret Service,
such as GNOME Keyring, must be running.
It doesn't unlock the keyring or prompt for a password;
the user unlocks it in the desktop session.

A process can request the value in two ways.


Command requests
~~~~~~~~~~~~~~~~

.. @artefact workshopctl get-secret

A command runs :command:`workshopctl get-secret <SDK>.<PLUG>`,
typically from a wrapper script that the SDK ships.
:program:`workshopctl` passes the request to |ws_markup| on the host,
which checks that the plug is connected,
looks up the item described by the connected slot,
and returns the value.
:program:`workshopctl` writes it to its standard output,
so the value goes only to the process that reads that output.

When the request fails,
:program:`workshopctl` prints a short message
that names the plug and the cause,
and its exit status tells the causes apart:
an unconnected plug, a locked keyring,
no matching item or several of them,
or another failure.
The :ref:`workshopctl get-secret reference <ref_workshopctl__cli>`
lists the exit statuses.
For a failure that |ws_markup| doesn't classify,
the message only says :samp:`internal error`,
because the underlying error can include details of the host or the keyring;
those details stay in the request's record on the host.


Service requests
~~~~~~~~~~~~~~~~

A long-running service receives the value as a systemd credential.
Every workshop runs a secret socket,
and runtime hooks get its path in :envvar:`SDK_SYSTEMD_SECRET_SOCKET`.
A hook that installs a unit
names the credential :samp:`{SDK}.{PLUG}`,
here :samp:`secret-demo.api-key`,
and points it at that socket:

.. code-block:: ini

   LoadCredential=secret-demo.api-key:${SDK_SYSTEMD_SECRET_SOCKET}


When systemd starts the unit,
it connects to the socket,
and the connection itself tells which unit asks for which credential.
A :samp:`workshop-secret@` service accepts the connection,
requests the value of :samp:`secret-demo.api-key` the same way a command does,
and returns it to systemd,
which places it in the unit's credentials directory.
The request happens on every start of the unit,
and only then.

A failed request doesn't stop the unit:
whether the plug isn't connected or the lookup fails,
systemd starts the unit with an empty credential,
and the cause is logged in the :samp:`workshop-secret@` journal.
This lets a service start without the secret
and decide for itself how to handle the missing value.

At launch,
a service can start before the user has connected the plug.
During an LXD container refresh,
intact SDK services restart with the workshop,
and updated SDKs run their :samp:`setup-base` hooks,
before |ws_markup| restores preserved connections.
A service that starts in either situation can therefore receive
an empty credential on that first start.
Give its unit a restart policy,
or restart it once the plug is connected;
:ref:`how_use_secrets` shows both.

VM refreshes don't restore interface connections.
Reconnect the plug,
then restart the service.


Request records
---------------

Every request,
whether from a command or a service,
creates a :samp:`Retrieve secret` change
named after the workshop, SDK, and plug,
such as :samp:`Retrieve secret "dev/secret-demo:api-key"`.
A :samp:`Done` change means that the value was delivered;
an :samp:`Error` change means that it wasn't,
and its task log names the cause.
Neither the change nor its tasks contain the value.

These records answer whether and when a secret was requested,
and why a request failed,
even when the SDK doesn't pass the error on.
They are troubleshooting history, not a durable audit trail,
because |ws_markup| prunes ready changes.
See :ref:`how_provide_secrets_check` for how to read them.


What a connection exposes
-------------------------

A connection grants access to the workshop,
not to one SDK or one process.
While the plug is connected,
any process in the workshop can request the value
by naming the plug's :samp:`{SDK}.{PLUG}`,
including commands that don't belong to the SDK
and units installed by other SDKs.

A systemd credential is readable by the unit's user and root.
Because ordinary workshop commands and services share the
:samp:`workshop` user,
other processes running as that user can read the credential file
while the service runs.

Once a process receives the value,
|ws_markup| has no further control over it.
The process can keep it in its environment,
pass it to its child processes,
or write it somewhere;
how far the value travels depends on how the SDK hands it on,
as compared in :ref:`how_use_secrets`.

Disconnect the plug when you don't need the secret,
especially before an autonomous coding agent starts working in the workshop.
Disconnecting stops new requests,
but it can't withdraw a value
that a process has already received.


See also
--------

Explanation:

- :ref:`exp_interface_concepts`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdk_definition`
- :ref:`exp_workshop_connection_lifecycle`
- :ref:`exp_workshop_definition`
- :ref:`security_coding_agents`


How-to guides:

- :ref:`how_provide_secrets`
- :ref:`how_use_secrets`


Reference:

- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition_interfaces`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshopctl__cli`
