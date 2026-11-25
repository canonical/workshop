.. _ref_sdk__cli:

.. meta::
   :description: Overview of the "sdk" CLI utility, listing available
                 commands and global options for managing SDKs.

sdk (CLI)
=========

The :program:`sdk` utility exposes the following commands,
each with its own set of options,
and also has a number of global flags:

-h, --help

   Print the help message for the command.


-v, --version

   Print SDK CLI version.


{{ range .Files }}
.. include:: {{ . }}

{{ end }}


.. _ref_sdk__cli_completion:

Shell completion
----------------

The :program:`sdk` CLI ships completion scripts
for Bash, Zsh, and Fish.

.. note::

   When |ws_markup| is installed via snap,
   completion for Bash, Zsh, and Fish is enabled automatically
   for both :program:`workshop` and :program:`sdk`;
   no further configuration is needed for these shells.


To enable completion for the current shell session,
source the script for your shell.

Bash:

.. code-block:: console

   $ source <(sdk completion bash)


Zsh:

.. code-block:: console

   $ source <(sdk completion zsh)


Fish:

.. code-block:: console

   $ sdk completion fish | source


For per-shell installation that persists across new sessions,
follow the instructions printed by the shell-specific help command.
For example, for Bash:

.. code-block:: console

   $ sdk completion bash --help


See also
--------

Explanation:

- :ref:`sdk CLI <exp_sdk_cli>`
