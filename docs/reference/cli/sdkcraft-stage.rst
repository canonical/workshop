.. _ref_sdkcraft_stage:


.. meta::
   :description: Reference documentation for the 'sdkcraft stage' command

sdkcraft stage
--------------

Stage built artifacts into a common staging area

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft stage [--destructive-mode | --use-lxd] [--shell | --shell-after] [--debug]
                      [--platform name | --build-for arch]
                      [part-name ...]

.. rubric:: Description


Stage built artifacts into a common staging area. If part names are
specified only those parts will be staged. The default is to stage
all parts.


.. rubric:: Flags


--destructive-mode

   Build in the current host
   Default: :samp:`False`


--use-lxd

   Build in a LXD container.
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

