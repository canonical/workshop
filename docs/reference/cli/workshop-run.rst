.. _ref_workshop_run:

workshop run
------------

Run a workshop script and wait for it to complete.

.. rubric:: Usage

.. code-block:: console

   $ workshop run [flags] [<WORKSHOP>] [--] <SCRIPT> <ARGUMENTS>...

.. rubric:: Description


The 'run' subcommand runs a script specified in the workshop definition file,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept a 'run' command, the workshop must be 'Ready' or 'Pending'.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'run' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the 'run' subcommand from the script and its arguments,
use shell syntax such as *--*:

$ workshop run nimble -- test --verbose

This syntax is required if the workshop name is omitted
and the script takes one or more arguments.

Notes:

- To start a workshop before running scripts in it, use 'workshop start'.

- You can set the working directory, environment variables, user and group ID
  for running the script in the workshop; reasonable defaults are provided.


.. rubric:: Examples


Run the 'build' script under the 'nimble' workshop
in the current project directory:

.. code-block:: console

   $ workshop run nimble build


A similar command that sets an environment variable and the working directory:

.. code-block:: console

   $ workshop run --env GO111MODULE=off -w /project nimble build


The workshop name is optional if the project only has one workshop:

.. code-block:: console

   $ workshop run build


Scripts can accept arguments,
if a separator or a workshop name is provided:

.. code-block:: console

   $ workshop run -- build --debug



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


