.. _ref_workshopctl__cli:


.. meta::
   :description: Command reference for 'workshopctl', detailing its use by SDKs
                 for reporting health and invoking subcommands in workshops.

workshopctl (CLI)
=================

.. @artefact workshopctl
.. @artefact SDK hook

SDKs use the :program:`workshopctl` tool when reporting to the workshop;
to invoke a subcommand, add it to your :ref:`SDK hook <ref_sdk_hooks>`.


workshopctl get-secret
----------------------

Get the value of a secret connected to the workshop.

.. rubric:: Usage

.. code-block:: console

   $ workshopctl get-secret [--systemd] <SDK>.<secret>


.. rubric:: Description

This command retrieves the value of a secret made available to an SDK
through a connected secret plug and writes it to standard output.
SDKs typically call it from wrapper scripts to inject secrets into
one-shot commands.

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 1 2 3

   * - Placeholder
     - Required
     - Value

   * - :samp:`<SDK>.<secret>`
     - Required unless :samp:`--systemd` is given.
     - The name of the SDK and of its secret plug, joined by a dot.


.. rubric:: Examples

Read a secret into an environment variable in a wrapper script:

.. code-block:: console

   $ API_KEY=$(workshopctl get-secret my-sdk.api-key)


.. rubric:: Flags

--systemd

   Service a systemd :samp:`LoadCredential` request.
   The workshop secret socket unit invokes :program:`workshopctl`
   with this flag, passing the accepted socket connection on
   standard input.
   The credential name in the requesting unit's
   :samp:`LoadCredential` entry must take the form
   :samp:`{<SDK>}.{<secret>}` so that the requesting SDK and secret
   can be identified from the connection, for example:

   .. code-block:: none

      LoadCredential=my-sdk.api-key:${SDK_SYSTEMD_SECRET_SOCKET}

   The secret value is delivered back to systemd through the
   connection.
   Not intended for interactive use.


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
     - Required when :samp:`<STATUS>` is :samp:`waiting` or :samp:`error`;
       not allowed with :samp:`okay`.
     - Arbitrary string explaining the status;
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

- :ref:`workshopctl CLI <exp_workshopctl_cli>`


Reference:

- :ref:`ref_sdk_hooks`
- :ref:`ref_workshop__cli`
