.. _ref_sdkcraft_login:


.. meta::
   :description: Reference documentation for the 'sdkcraft login' command

sdkcraft login
--------------

.. @artefact sdkcraft login

Log in to the SDK Store

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft login [--export FILE]

.. rubric:: Description


Log in to the SDK Store.

SDKcraft prompts for an Ubuntu One email address and password
(and a one-time password if two-factor authentication is enabled).

The login command requires a working keyring on the system it is used on.
As an alternative, set the :envvar:`SDKCRAFT_STORE_CREDENTIALS` environment
variable with exported credentials.

If ``--export`` is used, the credentials are written to the specified file
instead of the local keyring, and nothing is persisted locally.
This is suitable for CI/CD environments:

.. code-block:: console

   $ export SDKCRAFT_STORE_CREDENTIALS=$(cat <FILE>)


.. rubric:: Flags


--export FILE

   Export credentials to a file instead of the local keyring.
   The file is created with mode 0600.


.. rubric:: Examples


Log in interactively:

.. code-block:: console

   $ sdkcraft login

Export credentials to a file for use in CI pipelines:

.. code-block:: console

   $ sdkcraft login --export credentials.txt
