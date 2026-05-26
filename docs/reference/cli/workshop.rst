.. _ref_workshop__cli:

.. meta::
   :description: Overview of the "workshop" CLI utility, listing available
                 commands and global options for managing Workshop environments.

workshop (CLI)
==============

.. @artefact workshop (CLI)

The :program:`workshop` utility exposes the following commands,
each with its own set of options,
and also has a number of global flags:

-h, --help

   Print the help message for the command.


-p, --project

   Specify the project's directory path.


-v, --version

   Print Workshop version.



.. include:: workshop-actions.rst


.. include:: workshop-changes.rst


.. include:: workshop-connect.rst


.. include:: workshop-connections.rst


.. include:: workshop-disconnect.rst


.. include:: workshop-exec.rst


.. include:: workshop-info.rst


.. include:: workshop-init.rst


.. include:: workshop-launch.rst


.. include:: workshop-list.rst


.. include:: workshop-okay.rst


.. include:: workshop-refresh.rst


.. include:: workshop-remount.rst


.. include:: workshop-remove.rst


.. include:: workshop-restore.rst


.. include:: workshop-run.rst


.. include:: workshop-shell.rst


.. include:: workshop-sketch-sdk.rst


.. include:: workshop-sketches.rst


.. include:: workshop-start.rst


.. include:: workshop-stop.rst


.. include:: workshop-tasks.rst


.. include:: workshop-warnings.rst




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

- :ref:`exp_workshop_cli`
