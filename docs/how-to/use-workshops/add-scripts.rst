.. _how_add_scripts:

How to add scripts to your workshop
===================================

This guide explains how to add scripts to an existing workshop
to automate mundane tasks and enhance its functionality
without modifying the SDKs themselves
or running lengthy :command:`workshop exec` commands.


Adding scripts
--------------

To add scripts,
edit your :file:`workshop.yaml` file,
adding named script definitions in :program:`bash` format under :samp:`scripts`
and making use of the features provided by the SDKs in your workshop

Here's an example of a workshop definition with two scripts
that use the capabilities provided by the sketch SDK
from the :ref:`how_sketch` guide:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 7-11

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: jammy/stable
   
   scripts:
     lint: |
       golangci-lint run --out-format=colored-line-number -c .golangci.yaml
     shellcheck: |
       git ls-files | file --mime-type -Nnf- | grep shellscript | cut -f1 -d: | xargs shellcheck


Unlike changes in SDK layout or base,
script updates do not require a :command:`workshop refresh`.


Running scripts
---------------

To execute a script,
use the :command:`workshop run` command followed by the script name:

.. @artefact workshop run

.. code-block:: console

   $ workshop run lint

     main.go:1:
     ./main.go:5:2: "os" imported and not used (typecheck)
     package main

   $ workshop run shellcheck
   
     In 1.sh line 10:
     cat /etc/passwd | grep root
         ^---------^ SC2002 (style): Useless cat. Consider 'cmd < file | ..' or 'cmd file | ..' instead.


When you run a script using :command:`workshop run`,
any additional arguments provided after the script name
are passed directly to the script itself.


Conclusion
----------

By adding scripts to your workshop,
you can streamline your daily |ws_markup| workflows
and reduce the risk of typing errors.

For more advanced scripting capabilities,
consider exploring additional features of the SDKs,
such as hooks.


See also
--------

Explanation:

- :ref:`exp_workshop_definition_scripts`
- :ref:`exp_sdk_hooks`


Reference:

- :ref:`ref_workshop_exec`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_run`
- :ref:`ref_workshop_scripts`
