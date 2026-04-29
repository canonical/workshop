.. _ref_workshop_run:


.. meta::
   :description: Reference documentation for the 'workshop run' command

workshop run
------------

.. @artefact workshop run

Run a workshop action and wait for it to complete.

.. rubric:: Usage

.. code-block:: console

   $ workshop run [flags] [<WORKSHOP>] [--] <ACTION> <ARGUMENTS>...

.. rubric:: Description


The "run" subcommand runs an action specified in the workshop definition file,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept a "run" command, the workshop must be "Ready" or "Waiting".
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use "-i" or "-I". If neither is supplied,
"run" deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the "run" subcommand from the action and its arguments,
use a separator (*--*).

If you omit the separator,
"run" treats its first argument as the workshop name.
If the project has no such workshop
and the shell is interactive,
the argument is treated as an action to run in the default workshop.

Any trailing arguments are forwarded to the action as positional parameters,
so action scripts can consume them with standard shell expansions.

Notes:

- To start a workshop before running actions in it, use "workshop start".

- You can set the working directory, environment variables, user and group ID
  for running the action in the workshop; reasonable defaults are provided.


.. rubric:: Examples


Run the "build" action under the "nimble" workshop
in the current project directory:

.. code-block:: console

   $ workshop run nimble build


A similar command that sets an environment variable and the working directory:

.. code-block:: console

   $ workshop run --env GO111MODULE=off -w /project nimble -- build


If the project has only one workshop, the workshop name is optional:

.. code-block:: console

   $ workshop run -- build


If the action doesn't overlap with a workshop name
and the shell is interactive,
the separator is also optional:

.. code-block:: console

   $ workshop run build


Forward arguments to an action that consumes them
(for example, ``tests: go test "$@"`` in the workshop definition):

.. code-block:: console

   $ workshop run dev -- tests -run TestFoo ./pkg/...



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




