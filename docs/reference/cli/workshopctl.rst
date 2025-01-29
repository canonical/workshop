.. _ref_workshopctl:

workshopctl (CLI)
=================

.. @artefact workshopctl
.. @artefact SDK hook

SDKs use the :program:`workshopctl` tool when reporting to the workshop;
to invoke a subcommand, add it to your :ref:`SDK hook <ref_sdk_hooks>`.


workshopctl set-health
----------------------

Report the health of the SDK.

.. rubric:: Usage

.. code-block:: console

   $ workshopctl set-health [--code=<ERROR CODE>] <STATUS> [<MESSAGE>]


.. rubric:: Description

.. @artefact check-health

This command is essential for the :samp:`check-health` hook
that runs after launch or refresh operations in a workshop.
The arguments are as follows:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 1 2 3

   * - Placeholder
     - Required
     - Value

   * - :samp:`<STATUS>`
     - Required
     - Can be :samp:`okay`, :samp:`waiting` or :samp:`error`.

   * - :samp:`<MESSAGE>`
     - Required with :option:`!--code`
     - Arbitrary string explaining the context of the error code;
       7–70 characters.


.. rubric:: Examples

Report an error with a code and a message;
note only the message is quoted:

.. code-block:: console

   $ workshopctl set-health --code=missing-cuda error "CUDA libraries not found"


.. rubric:: Flags

--code

   Optional, can't go with :samp:`okay`.
   Short code of lowercase letters, hyphens and digits;
   3–30 characters, starts with a letter.


See also
--------

Explanation:

- :ref:`exp_workshopctl`


Reference:

- :ref:`ref_sdk_hooks`
- :ref:`ref_workshop_cli`
