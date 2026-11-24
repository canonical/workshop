.. _ref_sdkcraft_prime:


.. meta::
   :description: Reference documentation for the 'sdkcraft prime' command

sdkcraft prime
--------------

Prime artifacts defined for a part

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft prime [--destructive-mode | --use-lxd] [--shell | --shell-after] [--debug]
                      [--platform name | --build-for arch]
                      [part-name ...]

.. rubric:: Description


Prepare the final payload to be packed, performing additional
processing and adding metadata files. If part names are specified only
those parts will be primed. The default is to prime all parts.


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

