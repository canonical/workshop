.. _ref_sdkcraft_upload:


.. meta::
   :description: Reference documentation for the 'sdkcraft upload' command

sdkcraft upload
---------------

Upload an SDK artifact to the store

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft upload [--release CHANNELS] SDK

.. rubric:: Description


Upload an SDK artifact to the SDK Store.

The artifact must be a .sdk file created by the pack command.
Optionally, the uploaded revision can be released to specified channels.


.. rubric:: Flags


--release

   Comma-separated list of channels to release to after upload

