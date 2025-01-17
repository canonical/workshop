.. _ref_workshop_shell:

workshop shell
--------------

Start an interactive terminal session for the workshop.

.. rubric:: Usage

.. code-block:: console

   $ workshop shell [<WORKSHOP>] [flags]

.. rubric:: Description


The 'shell' subcommand runs an interactive terminal session
in the specified workshop.

To accept a 'shell' command, the workshop must be 'Ready' or 'Pending'.


Notes:

- To start a workshop before running a terminal session, use 'workshop start'.

- The subcommand is a shorthand for 'workshop exec';
  it launches the login shell for 'workshop',
  the default non-privileged user in a workshop.


.. rubric:: Examples


Open the default login shell of the 'workshop' user into the 'nimble' workshop
in the current project directory:

.. code-block:: console

   $ workshop shell nimble


The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop shell


