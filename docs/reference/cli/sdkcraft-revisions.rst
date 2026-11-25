.. _ref_sdkcraft_revisions:


.. meta::
   :description: Reference documentation for the 'sdkcraft revisions' command

sdkcraft revisions
------------------

List SDK revisions available on the store

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft revisions SDK

.. rubric:: Description


List all available channels and revisions for <sdk> from the store.

Use this command to find the revision number to pass to
`sdkcraft release <sdk> <revision> <channels>`.


.. rubric:: Examples


List revisions for an SDK:

.. code-block:: console

   $ sdkcraft revisions my-sdk

