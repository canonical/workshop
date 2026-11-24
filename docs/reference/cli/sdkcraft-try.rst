.. _ref_sdkcraft_try:


.. meta::
   :description: Reference documentation for the 'sdkcraft try' command

sdkcraft try
------------

Try SDKs before publishing

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft try [--destructive-mode] [--shell | --shell-after] [--debug]
                    [--platform name | --build-for arch] [--output OUTPUT]
                    [SDKs ...]

.. rubric:: Description


Pack the SDK and copy it to the Workshop try area.


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


--build-for

   Set architecture to build for


--output

   Output directory for created packages.
   Default: :samp:`.`


.. rubric:: Examples


Try the built artifact:

.. code-block:: console

   $ sdkcraft try

