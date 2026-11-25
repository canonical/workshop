.. _ref_sdkcraft_pull:


.. meta::
   :description: Reference documentation for the 'sdkcraft pull' command

sdkcraft pull
-------------

Download or retrieve artifacts defined for a part

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft pull [--destructive-mode | --use-lxd] [--shell | --shell-after] [--debug]
                     [--platform name | --build-for arch]
                     [part-name ...]

.. rubric:: Description


Download or retrieve artifacts defined for a part. If part names
are specified only those parts will be pulled, otherwise all parts
will be pulled.


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

