.. _ref_workshop_exec:


.. meta::
   :description: Reference documentation for the 'workshop exec' command

workshop exec
-------------

.. @artefact workshop exec

Run an ad-hoc shell command in a workshop.

.. rubric:: Usage

.. code-block:: console

   $ workshop exec [flags] [<WORKSHOP>] [--] <COMMAND>...

.. rubric:: Description

The "exec" subcommand runs an arbitrary command in the specified workshop,
waiting for it to complete. If a timeout elapses before that, it's terminated.

Use "exec" for one-off shell commands typed on the command line.
To invoke a named script defined in the workshop's "actions:" section instead,
use "workshop run".

To accept an "exec" command, the workshop must be "Ready" or "Waiting".
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use "-i" or "-I". If neither is supplied,
"exec" deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the "exec" subcommand from the command itself,
use a separator (*--*).

If you omit the separator,
"exec" treats its first argument as the workshop name.
If the project has no such workshop
and the shell is interactive,
the argument is treated as a command to run in the default workshop.

Notes:

- To start a workshop before running commands in it, use "workshop start".

- You can set the working directory, environment variables, user and group ID
  for running the command in the workshop; reasonable defaults are provided.

.. rubric:: Examples

Run the "go build main.go" command under the "nimble" workshop
in the current project directory:

.. code-block:: console

   $ workshop exec nimble -- go build main.go

A similar command that sets an environment variable and the working directory:

.. code-block:: console

   $ workshop exec --env GO111MODULE=off -w /project nimble -- go build -x

Run a command as root (the default is "workshop"):

.. code-block:: console

   $ workshop exec --uid 0 nimble id

Run a custom interactive shell:

.. code-block:: console

   $ workshop exec -I nimble sh

If the project has only one workshop, the workshop name is optional:

.. code-block:: console

   $ workshop exec -- sh

If the command doesn't overlap with a workshop name
and the shell is interactive,
the separator is also optional:

.. code-block:: console

   $ workshop exec sh


.. rubric:: Flags

-w, --cwd

   Set the working directory in the workshop.

   Default: ``/project``

--env

   Set an environment variable, e.g. 'FOO=bar'; if only the name is provided, the value is inherited from the CLI environment.

--uid

   Run as a specific workshop user.

   Default: ``1000``

--gid

   Run as a member of a specific workshop group.

   Default: ``1000``

--timeout

   Set a timeout; valid units are ns, us or µs, ms, s, m, h.

   Default: ``0s``

-i, --interactive

   Force interactive mode.

-I, --non-interactive

   Force non-interactive mode.

