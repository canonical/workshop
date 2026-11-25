.. _ref_workshop__cli:

.. meta::
   :description: Overview of the "workshop" CLI utility, listing available
                 commands and global options for managing Workshop environments.

workshop (CLI)
==============

The :program:`workshop` utility exposes the following commands,
each with its own set of options,
and also has a number of global flags:

-h, --help

   Print the help message for the command.


-p, --project

   Specify the project's directory path.


-v, --version

   Print Workshop version.


{{ range .Files }}
.. include:: {{ . }}

{{ end }}


.. _ref_workshop__cli_completion:

Shell completion
----------------

The :program:`workshop` CLI ships completion scripts
for Bash, Zsh, and Fish.

.. note::

   When |ws_markup| is installed via snap,
   completion for Bash, Zsh, and Fish is enabled automatically;
   no further configuration is needed for these shells.


To enable completion for the current shell session,
source the script for your shell.

Bash:

.. code-block:: console

   $ source <(workshop completion bash)


Zsh:

.. code-block:: console

   $ source <(workshop completion zsh)


Fish:

.. code-block:: console

   $ workshop completion fish | source


For per-shell installation that persists across new sessions,
follow the instructions printed by the shell-specific help command.
For example, for Bash:

.. code-block:: console

   $ workshop completion bash --help


What gets completed
~~~~~~~~~~~~~~~~~~~

Beyond subcommand and flag names,
the :program:`workshop` CLI completes arguments and flag values dynamically:

- Workshop names, filtered by lifecycle status per command;
  for example, :command:`workshop start` lists only *Stopped* workshops,
  while :command:`workshop stop` lists only *Ready* ones.

- Plugs and slots for :command:`workshop connect`
  and :command:`workshop disconnect`:
  the first argument completes available plugs,
  the second completes valid slots for the chosen plug.

- Recent change IDs for :command:`workshop tasks`.


See also
--------

Explanation:

- :ref:`workshop CLI <exp_workshop_cli>`
