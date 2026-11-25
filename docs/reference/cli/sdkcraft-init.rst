.. _ref_sdkcraft_init:


.. meta::
   :description: Reference documentation for the 'sdkcraft init' command

sdkcraft init
-------------

Initialize an SDKcraft project

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft init [--name NAME] [--profile {simple}] [project_dir]

.. rubric:: Description


Initialize an SDKcraft project by creating an 'sdkcraft.yaml' file
together with hooks and tests.


.. rubric:: Flags


--name

   The name of project; defaults to the name of <project_dir>


--profile

   Use the specified project profile (default is simple, choices are 'simple')
   Default: :samp:`simple`


.. rubric:: Examples


Initialize a new project:

.. code-block:: console

   $ sdkcraft init

