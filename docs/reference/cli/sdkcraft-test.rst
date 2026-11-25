.. _ref_sdkcraft_test:


.. meta::
   :description: Reference documentation for the 'sdkcraft test' command

sdkcraft test
-------------

Run SDK tests

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft test [--destructive-mode] [--shell | --shell-after] [--debug] [--platform name]
                     [--list]
                     [test_expressions ...]

.. rubric:: Description


Tests are defined and run using spread (https://github.com/canonical/spread).

Compared to running spread manually, `sdkcraft test` also:
- Packs SDKs for all platforms matching the current architecture.
- Copies the packed SDKs into the test environment using `sdkcraft try`.
- Installs Workshop in the test environment.
- Skips spread variants for bases that weren't packed.


.. rubric:: Flags


--destructive-mode

   Build in the current host
   Default: :samp:`False`


--shell

   Shell into the environment in lieu of the step to run.
   Default: :samp:`False`


--shell-after

   Shell into the environment after the step has run.
   Default: :samp:`False`


--debug

   Shell into the environment if the build fails.
   Default: :samp:`False`


--platform

   Set platform to build for


--list

   Just show list of jobs that would run.
   Default: :samp:`False`


.. rubric:: Examples


Test the project:

.. code-block:: console

   $ sdkcraft test


List the jobs that would run:

.. code-block:: console

   $ sdkcraft test --list


Run a specific test suite:

.. code-block:: console

   $ sdkcraft test my-suite/

