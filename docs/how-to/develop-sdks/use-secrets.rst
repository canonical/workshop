.. _how_use_secrets:

.. meta::
   :description: How-to guide for SDK authors on consuming credentials through
                 secret plugs, including on-demand helpers, command wrappers,
                 systemd credentials, failure handling, and exposure limits.

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
For the user-facing keyring, slot, and connection steps,
see :ref:`how_provide_secrets`.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- An existing |sdk_markup| project
  built with a version that supports :samp:`secret` plugs.

- A command, tool, or systemd service in the SDK
  that needs a credential at runtime.

- A test workshop in which the SDK's secret plug can be connected.
  Follow :ref:`how_provide_secrets`
  to configure a test item, slot, and connection.


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

   * - A value the user chooses for one invocation
     - The user's :option:`!--env` argument
     - The SDK remains usable without depending on user-specific state


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
Secret slots belong to the :samp:`system` SDK
and are added by the user in the workshop definition.

Inside the workshop,
the request identifier is :samp:`{SDK}.{PLUG}`;
for an SDK named :samp:`secret-demo`
with the plug above,
the identifier is :samp:`secret-demo.api-key`.

|ws_markup| never connects a secret plug automatically.
Document the plug name and the credential it expects
so users know which slot to create and connect.
The SDK must also handle the plug being unconnected
or the lookup failing.


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
the following check counts a deterministic 23-byte test value
without displaying it:

.. code-block:: console

   $ workshop exec dev -- sh -c 'workshopctl get-secret secret-demo.api-key | wc -c'

     23


When a request fails,
:program:`workshopctl` writes a short diagnostic to standard error
and exits with a status that identifies the failure.
For example,
an unconnected plug returns status 3:

.. code-block:: console

   $ workshop exec dev -- workshopctl get-secret secret-demo.api-key

     secret plug "secret-demo.api-key" is not connected


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

These records are recent troubleshooting history,
not a durable audit trail.
Ready changes are normally pruned after about 24 hours,
and older records can be removed sooner
when the daemon holds more than 500 ready changes.


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
the `Claude Code SDK <https://github.com/canonical/claude-code-sdk/>`_
uses Claude Code's :samp:`apiKeyHelper`.

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


With a connected plug,
verify that the tool receives the variable
without exporting it to the calling shell:

.. code-block:: console

   $ workshop exec dev -- sh -c 'secret-demo; echo "after: ${SECRET_DEMO_API_KEY-<unset>}"'

     secret-demo-tool: authenticated with a 23-character key
     after: <unset>


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


Each service start causes systemd to request the value again.
If the plug is unconnected or the lookup fails,
systemd still starts the unit with an empty credential.
Let the service reject that state,
and configure a restart policy or restart it after fixing the lookup.

Connection timing depends on the workshop runtime:

- On initial launch,
  the service can start before the user connects the plug.

- During an LXD container refresh,
  an updated SDK's :samp:`setup-base` hook runs
  before |ws_markup| restores preserved connections.
  The first service start can therefore receive an empty credential.

- A VM refresh does not restore interface connections.
  The user must reconnect the plug
  before a restarted service can receive the credential.


To request the value immediately after connecting the plug
or correcting the lookup,
restart the service:

.. code-block:: console

   $ workshop exec dev -- sudo systemctl restart secret-demo.service


Inspect both the service journal
and the :samp:`workshop-secret@` journal when delivery fails:

.. code-block:: console

   $ workshop exec dev -- sudo journalctl --no-pager -u secret-demo.service -u 'workshop-secret@*'


The credential lives in a systemd-managed file
for as long as the service runs.
The unit's user and root can read it;
other processes running as the same user can also read the file.
This is not process-level isolation.


Allow a user-supplied value for one invocation
----------------------------------------------

Users can pass a value from their calling environment
to one :command:`workshop exec` or :command:`workshop run` invocation:

.. code-block:: console

   $ workshop exec --env SECRET_DEMO_API_KEY dev -- secret-demo


Design the SDK so this remains an optional override.
The wrapper above gives an existing environment value precedence
and falls back to the connected plug when it is absent.
If the named variable is unset,
|ws_markup| skips it without an error.

Do not ask users to type the value itself into an argument.
They should pass only the variable name;
the value then comes from the environment
of the :program:`workshop` command.


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

   * - User-supplied value
     - The command and every process it starts
     - A variable exported by the user's shell or direnv
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
