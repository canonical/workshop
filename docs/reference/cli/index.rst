:hide-toc:

.. _ref_cli:

CLI commands
============

The :program:`workshop` utility exposes the following commands,
each with its own set of options,
and also has a number of global options
such as :option:`!--help` or :option:`!-h`.


.. include:: workshop-changes.rst


.. include:: workshop-connect.rst


.. include:: workshop-connections.rst


.. include:: workshop-disconnect.rst


.. include:: workshop-exec.rst


.. include:: workshop-hack.rst


.. include:: workshop-info.rst


.. include:: workshop-launch.rst


.. include:: workshop-list.rst


.. include:: workshop-okay.rst


.. include:: workshop-refresh.rst


.. include:: workshop-remount.rst


.. include:: workshop-remove.rst


.. include:: workshop-shell.rst


.. include:: workshop-start.rst


.. include:: workshop-stop.rst


.. include:: workshop-tasks.rst


.. include:: workshop-warnings.rst




Shell completion
----------------

To configure shell completion,
follow the instructions offered by :command:`workshop completion`:

.. code-block:: console

   $ workshop completion -h

For example, in your :file:`~/.bashrc` file:

.. code-block:: console

   $ source <(workshop completion bash)


See also
--------

Explanation:

- :ref:`exp_cli`
