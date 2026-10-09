.. _how_provide_secrets:

.. meta::
   :description: How-to guide on providing a credential to an SDK in a workshop,
                 either from the host keyring through a secret slot
                 and an explicitly connected plug,
                 or as an environment variable for a single command,
                 covering lookup failures and exposure limits.

How to provide secrets to a workshop
====================================

.. @artefact secret interface
.. @artefact workshop exec

Some SDKs need a credential at runtime,
such as an API key for an AI coding agent.
|ws_markup| can deliver that credential to the workshop
without writing the value into the workshop definition
or the project directory.

Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- A desktop session that runs a keyring service
  implementing the freedesktop.org Secret Service,
  such as GNOME Keyring.
  |ws_markup| looks the secret up through that session's D-Bus session bus.

- The :program:`secret-tool` utility,
  shipped in the :samp:`libsecret-tools` package on Ubuntu,
  to store and check keyring items.

- A workshop definition that includes an SDK declaring a :samp:`secret` plug.
  The SDK's documentation names the plug
  and the credential it expects.


The OpenAI API key in the examples below is illustrative.
Use the keyring attributes, slot name, and environment variable
expected by your SDK instead.


There are two ways to provide a credential,
so pick one before you start:

.. list-table::
   :header-rows: 1
   :widths: 1 2 2

   * -
     - From the host keyring
     - For a single command

   * - What persists
     - The value stays in your keyring;
       an LXD container workshop preserves the connection across refreshes
       until you disconnect it or remove an endpoint.
       A VM workshop requires reconnection after a refresh
     - Nothing; the next command in the workshop doesn't see the value

   * - Who can read the value
     - Any command in the workshop,
       through the connected plug,
       each time it asks for the value
     - The command you run and the processes it starts,
       for that run only

   * - Per-run effort
     - None once you connect the plug
     - Name the variable in every command

   * - Suits
     - An SDK you use repeatedly
     - A one-off command that you run yourself


.. warning::

   Don't put a secret in the workshop definition, its actions,
   project files, or shell profiles.
   These are plain files:
   the project directory, definition included,
   is visible inside the workshop and often ends up in version control,
   and a shell profile exports its variables to every command you run.


Use a secret from the host keyring
----------------------------------

The host keyring holds the value;
the workshop definition only says where to find it,
and connecting a plug decides whether the workshop can ask for it.


.. _how_provide_secrets_store:

Store the credential
~~~~~~~~~~~~~~~~~~~~

Store the credential in the host keyring
under attributes that identify it.
Attributes are name and value pairs you choose;
the workshop finds the item by them,
so pick a combination that no other item shares.
:program:`secret-tool` prompts for the value,
so it never appears on the command line:

.. code-block:: console

   $ secret-tool store --label="OpenAI API key" --collection=default service openai account work


To check that the item is stored without printing its value,
search by the same attributes and discard standard output.
:program:`secret-tool` writes the value there,
but prints the item's attributes on standard error:

.. code-block:: console

   $ secret-tool search service openai account work >/dev/null

     attribute.account = work
     attribute.service = openai


Add a secret slot
~~~~~~~~~~~~~~~~~

Describe where the secret lives
by adding a :samp:`secret` slot to the :samp:`system` SDK
in the workshop definition.
The slot carries lookup attributes only, never the value:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 4-10

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: system
       slots:
         openai-key:
           interface: secret
           attributes:
             service: openai
             account: work
     - name: <SDK>


Use only these keys in the slot:

- :samp:`attributes` holds at least one attribute with a string value.
  The lookup matches an item that carries all of them.

- :samp:`collection` optionally selects the keyring collection to search.
  Without it, the lookup searches the collection
  that the keyring's :samp:`default` alias points to,
  which is the Login keyring in GNOME Keyring.
  Any other value is matched against collection labels.
  The label must identify exactly one collection.

Apply the definition with :command:`workshop launch` for a new workshop,
or refresh an existing one:

.. code-block:: console

   $ workshop refresh


.. _how_provide_secrets_connect:

Connect the plug
~~~~~~~~~~~~~~~~

|ws_markup| never connects a :samp:`secret` plug on its own:
after you add the slot and launch or refresh,
the plug and the slot stay unconnected until you connect them:

.. code-block:: console

   $ workshop connections <WORKSHOP>

     INTERFACE  PLUG                     SLOT                          NOTES
     ...
     secret     -                        <WORKSHOP>/system:openai-key  -
     secret     <WORKSHOP>/<SDK>:<PLUG>  -                             -


Connect the SDK's plug to the slot
to let the workshop ask the keyring for the secret:

.. code-block:: console

   $ workshop connect <WORKSHOP>/<SDK>:<PLUG> <WORKSHOP>/system:openai-key


Confirm the connection in the connections listing:

.. code-block:: console

   $ workshop connections <WORKSHOP>

     INTERFACE  PLUG                     SLOT                          NOTES
     ...
     secret     <WORKSHOP>/<SDK>:<PLUG>  <WORKSHOP>/system:openai-key  manual


For an LXD container workshop,
the connection persists across :command:`workshop refresh`,
including when the refresh changes the slot's attributes.
VM workshops don't restore interface connections during a refresh;
reconnect the plug afterward.
To withdraw access, disconnect the plug:

.. code-block:: console

   $ workshop disconnect <WORKSHOP>/<SDK>:<PLUG>


Use the secret
~~~~~~~~~~~~~~

Run the SDK's commands as usual.
Nothing is fetched in advance:
|ws_markup| looks the value up in the host keyring
only when an SDK command or service requests it,
and keeps no copy in the workshop.
A direct command receives the value on standard output.
A systemd service receives it in a credential file
that the unit's user and root can read;
other workshop processes running as the same user can also read that file.
Each request is a fresh lookup,
so a value you change in the keyring takes effect on the next one.
While the plug stays connected,
any command in the workshop can request the value the same way.


.. _how_provide_secrets_check:

Check when the secret was requested
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Every request creates a recent record in the workshop's changes,
whether it comes from an SDK command or a service,
so you can check whether and when the SDK received the secret.
:command:`workshop changes` lists each request
as a :samp:`Retrieve secret` change named after the plug,
among the other recent changes:

.. code-block:: console

   $ workshop changes

     ID  STATUS  SPAWN               READY               SUMMARY
     ...
     44  Done    today at 12:11 UTC  today at 12:11 UTC  Execute command "sh"
     45  Error   today at 12:11 UTC  today at 12:11 UTC  Retrieve secret "<WORKSHOP>/<SDK>:<PLUG>"
     ...
     58  Done    today at 12:12 UTC  today at 12:12 UTC  Execute command "sh"
     59  Done    today at 12:12 UTC  today at 12:12 UTC  Retrieve secret "<WORKSHOP>/<SDK>:<PLUG>"


A :samp:`Done` change means that the secret was delivered
to the process that requested it;
:samp:`Error` means that it wasn't.
The :samp:`SPAWN` column shows when the request was made.
Neither the changes nor their tasks ever contain the value.
They are troubleshooting history, not a durable audit trail:
ready changes are normally pruned after about 24 hours,
and older records can be removed sooner
when the daemon holds more than 500 ready changes.

To see why a request failed,
list the tasks of its change;
the end of the logged error names the cause:

.. code-block:: console

   $ workshop tasks 45

     STATUS  DURATION  SUMMARY
     Error       47ms  Retrieve secret "<WORKSHOP>/<SDK>:<PLUG>"

     ......................................................................
     Retrieve secret "<WORKSHOP>/<SDK>:<PLUG>"

     2026-10-09T12:11:46Z ERROR getting secret value for sdk "<SDK>" and plug "<PLUG>" in workshop "<WORKSHOP>": resolving secret: provider "system": retrieving system secret: secret provider is locked


While |ws_markup| retains the change,
the log is available even when the SDK doesn't show you the error.


Fix failed lookups
~~~~~~~~~~~~~~~~~~

A connection doesn't guarantee that the lookup succeeds,
because |ws_markup| asks the keyring only when the value is requested.
How a failure surfaces depends on the SDK.
To see the reason,
request the value yourself and discard it,
so that only an error reaches your terminal:

.. code-block:: console

   $ workshop exec <WORKSHOP> -- sh -c 'workshopctl get-secret <SDK>.<PLUG> > /dev/null'

     cannot retrieve secret for plug "<SDK>.<PLUG>": unlock the secret provider and try again


No output means the lookup succeeded.
Otherwise, the error names the cause:

.. list-table::
   :header-rows: 1
   :widths: 3 3 4

   * - Error
     - Cause
     - Fix

   * - :samp:`cannot retrieve secret for plug "{SDK}.{PLUG}": unlock the secret provider and try again`
     - The keyring collection is locked;
       logged as :samp:`secret provider is locked`.
     - Unlock the keyring in your desktop session,
       then run the command again.

   * - :samp:`no secret found for plug "{SDK}.{PLUG}"`
     - No item in the collection carries all the slot's attributes,
       or the slot names a collection that doesn't exist;
       logged as :samp:`secret not found`.
     - Compare the slot with :command:`secret-tool search`
       using the same attributes,
       correct the slot's :samp:`attributes` or :samp:`collection`,
       then run :command:`workshop refresh`;
       an LXD container keeps the connection,
       but a VM workshop must be reconnected.

   * - :samp:`multiple secrets match plug "{SDK}.{PLUG}"; refine the secret slot`
     - Several items carry all the slot's attributes;
       logged as :samp:`multiple secrets match the request`.
     - Add an attribute to the slot that only the intended item carries,
       then run :command:`workshop refresh`,
       or remove the other items from the keyring.
       Reconnect the plug after refreshing a VM workshop.

   * - :samp:`secret plug "{SDK}.{PLUG}" is not connected`
     - The plug isn't connected to the slot;
       logged as :samp:`plug is not connected`.
     - Connect it as described in :ref:`how_provide_secrets_connect`.

   * - :samp:`cannot retrieve secret for plug "{SDK}.{PLUG}": internal error`
     - Another failure;
       the error leaves out the details.
     - Find the cause in the logged error,
       as described in :ref:`how_provide_secrets_check`.
       If the log reports an ambiguous collection label,
       rename the collection or select a label that identifies only one.


Pass a secret to a single command
---------------------------------

For a one-off command that you run yourself,
pass the value as an environment variable
of a single :command:`workshop exec` or :command:`workshop run` invocation
with the :option:`!--env` flag.

Don't type the value itself on the command line,
as in :samp:`--env OPENAI_API_KEY={VALUE}`:
it stays in your shell history,
and other processes on the host can read it from the process list
while the command runs.
Instead, give :option:`!--env` only the variable's name;
|ws_markup| then takes the value from the environment
of the :program:`workshop` command itself.
Fill that variable from where the credential is already stored,
so that you never type the value.

For a single command,
fetch the value with a command substitution
in an assignment that prefixes the command:

.. code-block:: console

   $ OPENAI_API_KEY=$(secret-tool lookup service openai account work) workshop exec --env OPENAI_API_KEY <WORKSHOP> -- <COMMAND>


The shell history records the lookup, not the value,
and the assignment applies only to that command,
so the variable doesn't remain in your shell afterwards.

If you use `direnv <https://direnv.net/>`_ on the host,
let it set the variable whenever you enter the project directory,
with an :file:`.envrc` file that runs the same lookup:

.. code-block:: shell
   :caption: .envrc

   export OPENAI_API_KEY=$(secret-tool lookup service openai account work)


Then name the variable in each command:

.. code-block:: console

   $ workshop exec --env OPENAI_API_KEY <WORKSHOP> -- <COMMAND>


The :file:`.envrc` file lives in the project directory,
which is visible inside the workshop and may end up in version control,
so it must hold only the lookup, never the value.
Also, every command you run in that directory on the host
inherits the variable.

If the named variable isn't set,
|ws_markup| skips it without an error
and runs the command without it.

In both cases, the value exists only for that invocation:
the command and every process it starts can read it,
and the next command in the workshop doesn't see it.


See also
--------

Explanation:

- :ref:`exp_plugs_slots`
- :ref:`exp_secret_interface`


Reference:

- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition_interfaces`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_exec`
- :ref:`ref_workshop_run`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshopctl__cli`
