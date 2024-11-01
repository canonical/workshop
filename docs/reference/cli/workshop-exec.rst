.. _ref_workshop_exec:

workshop exec
-------------

Run a command and wait for it to complete

Synopsis
--------


The 'exec' subcommand runs an arbitrary command in the specified workshop,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept an 'exec' command, the workshop must be *Ready* or *Pending*.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'exec' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the 'exec' subcommand from the command itself,
use shell syntax such as '--':

  $ workshop exec nimble -- echo -n foo bar


Notes:

- To start a workshop before running commands in it, use 'workshop start'

- You can set the working directory, environment variables, user and group ID
  for running the command in the workshop; reasonable defaults are provided


.. code-block:: console

   workshop exec <WORKSHOP> [flags]


Options
-------

.. code-block:: console

  -w, --cwd string         Set the working directory in the workshop (default "/project")
      --env stringArray    Set an environment variable, e.g. 'FOO=bar'; if only the name is provided, the value is inherited from the CLI environment.
      --uid int            Run as a specific workshop user (default 1000)
      --gid int            Run as a member of a specific workshop group (default 1000)
      --timeout duration   Set a timeout; valid units are 'ns', 'us'/'µs', 'ms', 's', 'm', 'h'
  -i, --interactive        Force interactive mode
  -I, --non-interactive    Force non-interactive mode

