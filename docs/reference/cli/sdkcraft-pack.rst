.. _ref_sdkcraft_pack:


.. meta::
   :description: Reference documentation for the 'sdkcraft pack' command

sdkcraft pack
-------------

Create the final artifact

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft pack [--destructive-mode] [--shell | --shell-after] [--debug]
                     [--platform name | --build-for arch] [--output OUTPUT]

.. rubric:: Description


Process parts and create the final artifact.


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


Pack the project:

.. code-block:: console

   $ sdkcraft pack


Pack to a specific output directory:

.. code-block:: console

   $ sdkcraft pack --output dist/

