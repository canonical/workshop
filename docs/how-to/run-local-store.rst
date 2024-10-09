.. _how_run_local_store:

How to run a local SDK store
============================

To test SDKs locally without publishing we can run
a local instance of the SDK store. This guide uses
the open-source `fake-gcs-server <https://github.com/fsouza/fake-gcs-server>`_

.. note::

   This guide assumes that you are familiar with SDKcraft and have
   a target SDK in mind.


Create the directory structure
------------------------------

The SDK store uses directory structure to determine
SDK names and channels. Because of this, when running
a store locally we must make sure our directory structure
mirrors the real store.

You can call the 'fake-store' directory whatever you
wish, however the remaining structure and naming
convention is required

.. code-block:: console

   $ mkdir -p fake-store/sdk-store/<sdk>/<release>/<channel>


Where:
- :samp:`<sdk>` is the SDK name (ie. :samp:`my-sdk`)
- :samp:`<release>` is the sdk release (ie. :samp:`latest`)
- :samp:`<channel>` is the sdk channel (ie. :samp:`edge`)


Copying the SDK
---------------
Place the SDK in the deepest directory created at the previous step.
(ie. :file:`fake-store/sdk-store/my-sdk/latest/edge/my-sdk/`) and the
corresponding SDK definition (ie. my-sdk.yaml) should be renamed
to sdk.yaml and placed in the same location.

.. code-block:: console

   $ ls fake-store/sdk-store/my-sdk/latest/edge

     my-sdk.sdk  sdk.yaml


Running the local store
-----------------------

.. code-block:: console

   $ go run github.com/fsouza/fake-gcs-server@latest -data <path-to-dir-containing-sdk-
   store> -filesystem-root <same-as-data> -scheme http -port 8080 -public-host localhost:8080

     time=1990-01-01T00:00:00.000+00.00 level=INFO msg="server started at http://0.0.0.0:8080"


Using the local store with Workshop
-----------------------------------

Override the workshop URL:
.. code-block:: console

   $ sudo snap set workshop store.url=http://localhost:8080/storage/v1/
   $ sudo snap restart workshop


That's it! Workshop will now pull from your local store.


Reverting changes
-----------------

To go back to the default store:

.. code-block:: console

   $ sudo snap set workshop store.url=""


Workshop will now use the default URL.
