.. _ref_workshop_exec:

workshop exec
-------------

Run a command and wait for it to complete.

.. rubric:: Usage

.. code-block:: console

   $ workshop exec [flags] [<WORKSHOP>] [--] <COMMAND>...

.. rubric:: Description


The 'exec' subcommand runs an arbitrary command in the specified workshop,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept an 'exec' command, the workshop must be 'Ready' or 'Pending'.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'exec' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the 'exec' subcommand from the command itself,
use shell syntax such as *--*:

  $ workshop exec nimble -- echo -n foo bar

This syntax is required if the workshop name is omitted.

Notes:

- To start a workshop before running commands in it, use 'workshop start'.

- You can set the working directory, environment variables, user and group ID
  for running the command in the workshop; reasonable defaults are provided.


.. rubric:: Examples


Run the 'go build main.go' command under the 'nimble' workshop
in the current project directory:

.. code-block:: console

   $ workshop exec nimble go build main.go


A similar command that sets an environment variable and the working directory:

.. code-block:: console

   $ workshop exec --env GO111MODULE=off -w /project nimble go build -x


Run a custom interactive shell:

.. code-block:: console

   $ workshop exec -I nimble sh


The name is optional if the project has only one workshop
and a separator is provided:

.. code-block:: console

   $ workshop exec -I -- sh


Run a command as root (the default is 'workshop'):

.. code-block:: console

   $ workshop exec --uid 0 nimble id



.. rubric:: Flags


--cwd

   Set the working directory in the workshop.


--env

   Set an environment variable, e.g. 'FOO=bar'; if only the name is provided, the value is inherited from the CLI environment.


--uid

   Run as a specific workshop user.


--gid

   Run as a member of a specific workshop group.


--timeout

   Set a timeout; valid units are ns, us or µs, ms, s, m, h.


--interactive

   Force interactive mode.


--non-interactive

   Force non-interactive mode.


