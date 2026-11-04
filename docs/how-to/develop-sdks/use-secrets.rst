.. _how_use_secrets:

.. meta::
   :description: How-to guide for SDK authors on consuming credentials through
                 secret plugs, from a test connection to on-demand helpers,
                 command wrappers, systemd credentials, single-command values,
                 failure handling, and exposure limits.

How to use secrets in an SDK
============================

.. @tests in tests/docs-how-to/use-secrets/task.yaml

.. @artefact secret interface
.. @artefact workshopctl get-secret

An SDK that needs a credential at runtime,
such as an API key,
declares a :samp:`secret` plug.
After a user connects the plug to a secret slot,
the SDK can request the value when it is needed
without shipping or storing the value itself.

Use this guide to add one of the supported delivery patterns
to an existing SDK.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- An existing |sdk_markup| project.

- A command, tool, or systemd service in the SDK
  that needs a credential at runtime.

- A host keyring and the :program:`secret-tool` utility
  to store a test value,
  as listed in :ref:`how_provide_secrets`.


Choose how the SDK receives the value
-------------------------------------

Choose the delivery mechanism based on what consumes the credential:

.. list-table::
   :header-rows: 1
   :widths: 2 3 4

   * - What needs it
     - Mechanism
     - Why

   * - A tool with a credential-helper setting
     - :command:`workshopctl get-secret`,
       called directly by the tool
     - The tool requests the value only when needed
       without putting it in the tool's environment

   * - A command that reads standard input
     - Pipe from :command:`workshopctl get-secret`
     - The value can stay out of the command's environment

   * - A command that requires an environment variable
     - An SDK-supplied wrapper
     - The variable exists only in the wrapper and the process it starts

   * - A long-running systemd service
     - :samp:`LoadCredential=` in the service unit
     - systemd requests the value on each start
       and makes it available as a credential file

   * - A value the user passes for a single command
     - The user's :option:`!--env` argument
     - The user supplies the value ad hoc,
       without a slot or a connection


.. warning::

   Never put a secret in SDK content, :file:`sdkcraft.yaml`,
   workshop definitions, hooks, shell profiles, examples, or logs.
   Each of these is a file that ships, persists, or is shared,
   so a value written there outlives the moment it was needed.


Declare a secret plug
---------------------

Add a :samp:`secret` plug to :file:`sdkcraft.yaml`.
The plug has a name and an interface,
but no lookup attributes or value:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   name: secret-demo
   # ...
   plugs:
     api-key:
       interface: secret


Only regular SDKs can declare secret plugs.

Inside the workshop,
the request identifier is :samp:`<SDK>.<PLUG>`;
for an SDK named :samp:`secret-demo`
with the plug above,
the identifier is :samp:`secret-demo.api-key`.

|ws_markup| never connects a secret plug automatically.
Document the plug name and the credential it expects
so users know which slot to create and connect.
The SDK must also handle the plug being unconnected
or the lookup failing.


Connect the plug in a test workshop
-----------------------------------

The plug has nothing to request
until it's connected to a secret slot.
Slots don't ship with the SDK:
they belong to the :samp:`system` SDK,
and each user adds one to their workshop definition,
with lookup attributes that identify an item in their own keyring.
To test the SDK, take the same steps with a throwaway value.

Store the test value in the host keyring.
:program:`secret-tool` prompts for the value;
enter the 23-byte :samp:`not-a-real-secret-value`
to match the output of later steps:

.. code-block:: console

   $ secret-tool store --label="Secret demo test key" --collection=default service secret-demo account test


Build and install the SDK with :command:`sdkcraft try`,
as described in :ref:`how_build_sdk_try`:

.. code-block:: console

   $ sdkcraft try


In the test workshop definition,
add a :samp:`secret` slot to the :samp:`system` SDK
with attributes that match the stored item,
and list the SDK with the :samp:`try-` prefix:

.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: system
       slots:
         demo-key:
           interface: secret
           attributes:
             service: secret-demo
             account: test
     - name: try-secret-demo


Launch the workshop:

.. code-block:: console

   $ workshop launch


Then connect the plug to the slot.
The plug keeps the SDK's own name, without the :samp:`try-` prefix:

.. code-block:: console

   $ workshop connect dev/secret-demo:api-key dev/system:demo-key


The connection persists when you rebuild the SDK
and refresh the workshop in later steps.


Request a secret on demand
--------------------------

Run :command:`workshopctl get-secret <SDK>.<PLUG>`
inside the workshop to request a connected secret:

.. code-block:: console

   $ workshopctl get-secret <SDK>.<PLUG>


The command writes the value to standard output.
Capture or pipe that output directly into the consuming tool;
do not print it to a terminal or log.

For example,
the following check counts the bytes of the test value
without displaying it:

.. code-block:: console

   $ workshop exec dev -- workshopctl get-secret secret-demo.api-key | wc -c

     23


When a request fails,
:program:`workshopctl` writes a short diagnostic to standard error
and exits with a status that identifies the failure.
To see one, disconnect the plug:

.. code-block:: console

   $ workshop disconnect dev/secret-demo:api-key


Then repeat the request;
an unconnected plug returns status 3:

.. code-block:: console

   $ workshop exec dev -- workshopctl get-secret secret-demo.api-key

     secret plug "secret-demo.api-key" is not connected


Reconnect the plug before you continue:

.. code-block:: console

   $ workshop connect dev/secret-demo:api-key dev/system:demo-key


The :ref:`workshopctl get-secret reference <ref_workshopctl__cli>`
lists every exit status.
Branch on the status when the SDK needs to distinguish
an unavailable provider, a missing item, multiple matches,
or an unconnected plug.

Every request also creates a :samp:`Retrieve secret` change.
The change and its tasks record whether delivery succeeded and why it failed,
but never contain the value.
Use :command:`workshop changes` and :command:`workshop tasks`
to troubleshoot an integration that hides the immediate error.


Use a tool-native credential helper
-----------------------------------

Prefer a tool-native credential helper when one is available.
Configure the helper to run:

.. code-block:: console

   $ workshopctl get-secret <SDK>.<PLUG>


The tool controls when it calls the helper
and how long it retains the returned value.
The value does not need to enter the tool's environment.

For example,
Claude Code's :samp:`apiKeyHelper` setting runs a command
and uses its output as the API key.

If the tool accepts the credential on standard input instead,
pipe the command output directly into the tool.
Keep shell tracing such as :samp:`set -x` disabled,
because it can print assignments and command substitutions.


Wrap a tool that requires an environment variable
-------------------------------------------------

When a tool reads its credential only from an environment variable,
ship a wrapper that requests the value
and exports it only for the tool process.

The following wrapper starts :file:`bin/secret-demo-tool`
with :envvar:`SECRET_DEMO_API_KEY`.
A value already supplied by the user takes precedence:

.. code-block:: shell
   :caption: files/bin/secret-demo

   #!/usr/bin/bash
   # Runs secret-demo-tool with the API key from the connected secret plug.
   # A key already set in the environment takes precedence.
   sdk_dir=$(dirname "$(dirname "$(readlink -f "$0")")")

   if [[ -z "${SECRET_DEMO_API_KEY:-}" ]]; then
     if key=$(workshopctl get-secret secret-demo.api-key); then
       export SECRET_DEMO_API_KEY="$key"
     fi
     unset key
   fi

   exec "$sdk_dir/bin/secret-demo-tool" "$@"


If the request fails,
:command:`workshopctl get-secret` already writes the diagnostic.
The wrapper starts the tool without the variable,
allowing the tool to use another login mechanism
or report that the credential is missing.

If a silent fallback is intentional,
redirect standard error only for the request:

.. code-block:: shell

   if key=$(workshopctl get-secret secret-demo.api-key 2>/dev/null); then
     export SECRET_DEMO_API_KEY="$key"
   fi


The corresponding :samp:`Retrieve secret` change
remains available for troubleshooting while |ws_markup| retains it.

Install the wrapper on :envvar:`PATH`
from the SDK's :samp:`setup-base` hook:

.. code-block:: shell
   :caption: hooks/setup-base

   #!/usr/bin/bash
   cat >/etc/profile.d/secret-demo.sh <<PROFILE
   export PATH="${SDK}/bin:\$PATH"
   PROFILE


Rebuild the SDK to include the wrapper and the hook:

.. code-block:: console

   $ sdkcraft try


Refresh the test workshop to install the rebuilt SDK:

.. code-block:: console

   $ workshop refresh


Then check that the tool receives the key:

.. code-block:: console

   $ workshop exec dev -- secret-demo

     secret-demo-tool: authenticated with a 23-character key


Also test the unavailable path
by disconnecting the plug or locking the provider.
The wrapper must not turn a failed lookup into a success-shaped result.


Deliver a secret to a systemd service
-------------------------------------

Use a systemd credential for a long-running service.
The service reads the credential from the file
named after it in :envvar:`CREDENTIALS_DIRECTORY`.

For example,
this service process rejects a missing or empty credential
before starting its long-running work:

.. code-block:: shell
   :caption: files/bin/secret-demo-service

   #!/usr/bin/bash
   # Reads the API key from a systemd credential, then runs the service.
   key_file="$CREDENTIALS_DIRECTORY/secret-demo.api-key"
   if [[ ! -s "$key_file" ]]; then
     echo "secret-demo-service: no API key in the secret-demo.api-key credential" >&2
     exit 1
   fi
   echo "secret-demo-service: started with a $(wc -c <"$key_file")-byte key"
   exec sleep infinity


Install the unit from :samp:`setup-base`.
Runtime hooks receive :envvar:`SDK_SYSTEMD_SECRET_SOCKET`,
which is the socket systemd uses to request the value:

.. code-block:: shell
   :caption: hooks/setup-base

   #!/usr/bin/bash
   cat >/etc/profile.d/secret-demo.sh <<PROFILE
   export PATH="${SDK}/bin:\$PATH"
   PROFILE

   cat >/etc/systemd/system/secret-demo.service <<UNIT
   [Unit]
   Description=Secret demo service

   [Service]
   User=workshop
   LoadCredential=secret-demo.api-key:${SDK_SYSTEMD_SECRET_SOCKET}
   ExecStart=${SDK}/bin/secret-demo-service
   Restart=on-failure
   RestartSec=30

   [Install]
   WantedBy=multi-user.target
   UNIT

   systemctl daemon-reload
   systemctl enable --now secret-demo.service


Rebuild the SDK to include the service and the updated hook:

.. code-block:: console

   $ sdkcraft try


Refresh the test workshop to install the rebuilt SDK
and start the service:

.. code-block:: console

   $ workshop refresh


Each service start causes systemd to request the value again.
If the plug is unconnected or the lookup fails,
systemd still starts the unit with an empty credential.
Let the service reject that state,
and configure a restart policy or restart it after fixing the lookup.

Expect an empty credential in two cases:

- On initial launch,
  the service can start before the user connects the plug.

- During a refresh,
  an updated SDK's :samp:`setup-base` hook runs
  before |ws_markup| restores preserved connections.
  The first service start can therefore receive an empty credential.


The refresh you just ran is the second case.
To request the value immediately,
whether after a refresh, after connecting the plug,
or after correcting the lookup,
restart the service:

.. code-block:: console

   $ workshop exec dev -- sudo systemctl restart secret-demo.service


Then check the service journal.
The start during the refresh rejected the empty credential,
and the restart received the key:

.. code-block:: console

   $ workshop exec dev -- sudo journalctl --no-pager -u secret-demo.service

     ...
     Oct 13 10:41:57 dev secret-demo-service[488]: secret-demo-service: no API key in the secret-demo.api-key credential
     ...
     Oct 13 10:42:01 dev secret-demo-service[522]: secret-demo-service: started with a 23-byte key


When delivery fails,
inspect both the service journal
and the :samp:`workshop-secret@` journal:

.. code-block:: console

   $ workshop exec dev -- sudo journalctl --no-pager -u secret-demo.service -u 'workshop-secret@*'


The credential lives in a systemd-managed file
for as long as the service runs.
The unit's user and root can read it;
other processes running as the same user can also read the file.
This is not process-level isolation.


Accept a secret for a single command
------------------------------------

Users can also pass a credential ad hoc
to a single :command:`workshop exec` or :command:`workshop run` invocation,
without a slot or a connection.
The value stays off the command line:
users give the :option:`!--env` flag only the variable's name
and fill the variable from a credential store on the host
with a command substitution:

.. code-block:: console

   $ SECRET_DEMO_API_KEY=$(secret-tool lookup service secret-demo account test) workshop exec --env SECRET_DEMO_API_KEY dev -- secret-demo

     secret-demo-tool: authenticated with a 23-character key


Accept the value in the variable that the wrapper exports for the tool,
so that an ad hoc value overrides the connected plug:
the wrapper above gives an existing value precedence
and requests the connected secret only when the variable is absent.
If the named variable is unset,
|ws_markup| skips it without an error,
and the wrapper falls back to the plug.

Document the variable's name for users,
and tell them to pass only the name.
A value typed into :samp:`--env <NAME>=<VALUE>`
stays in the shell history,
and other processes on the host can read it from the process list
while the command runs.


Review the exposure limits
--------------------------

Each delivery mechanism keeps the value out of SDK content,
but the value remains readable somewhere while it is in use:

.. list-table::
   :header-rows: 1
   :widths: 2 4 4

   * - Mechanism
     - What can read the value
     - What the SDK cannot clean up

   * - Tool-native helper or standard input
     - The process that consumes the command output
       and anything to which that process passes it
     - While the plug is connected,
       any process in the workshop can make a new request

   * - Command wrapper
     - The tool and every process it starts,
       through the tool's environment
     - While the plug is connected,
       any process in the workshop can request the value
       with :command:`workshopctl get-secret`

   * - Systemd credential
     - The unit's user, root,
       and other processes running as that user
     - The credential file remains available
       until systemd stops the service

   * - Single-command value
     - The command and every process it starts
     - A variable exported by the user's shell
       or by a tool such as direnv
       stays available to every command run from that environment


Disconnecting a plug prevents new requests,
but it cannot withdraw a value
that a process has already received.
Document this limitation for users,
especially when the SDK can run autonomous tools.


See also
--------

Explanation:

- :ref:`exp_secret_interface`


How-to guides:

- :ref:`how_declare_plugs_slots`
- :ref:`how_provide_secrets`
- :ref:`how_write_runtime_hooks`


Reference:

- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_definition_interfaces`
- :ref:`ref_workshop_exec`
- :ref:`ref_workshop_run`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshopctl__cli`
