.. _ref_sdkcraft_clean:


.. meta::
   :description: Reference documentation for the 'sdkcraft clean' command

sdkcraft clean
--------------

Remove a part's assets

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft clean [--destructive-mode] [--platform name] [part-name ...]

.. rubric:: Description


Clean up artifacts belonging to parts. If no parts are specified,
remove the packing environment.


.. rubric:: Flags


--destructive-mode

   Build in the current host
   Default: :samp:`False`


--platform

   Platform to clean


.. rubric:: Examples


Clean build artifacts:

.. code-block:: console

   $ sdkcraft clean


Clean specific parts:

.. code-block:: console

   $ sdkcraft clean my-part


Clean in destructive mode:

.. code-block:: console

   $ sdkcraft clean --destructive-mode

