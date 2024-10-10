.. _how_run_local_store:

How to run a local SDK Store
============================

To test SDKs with |project_markup| locally without publishing,
it is possible to run a local instance of SDK Store.
This guide uses the open-source `fake-gcs-server <https://github.com/fsouza/fake-gcs-server>`_.

.. note::

   This guide assumes you're familiar with `SDKcraft`_.


Create the directory structure
------------------------------

SDK Store relies on a directory structure
to determine SDK names and channels.
Therefore, when running a store locally,
the directory structure must mirror that of the real store.

The 'fake store' directory can be named as preferred;
however, the remainder of the structure and naming convention is mandatory.

.. code-block:: console

   $ mkdir -p fake-store/sdk-store/<SDK>/<RELEASE>/<CHANNEL>

Here:

- :samp:`<SDK>` is the SDK name (e.g. :samp:`my-sdk`)

- :samp:`<RELEASE>` is the SDK release (e.g. :samp:`latest`)

- :samp:`<CHANNEL>` is the SDK channel (e.g. :samp:`edge`)


Copy the SDK
------------

Place the SDK files in the deepest directory from the previous step
(e.g. :file:`fake-store/sdk-store/my-sdk/latest/edge/my-sdk/`).
Rename the SDK definition (e.g. :file:`my-sdk.yaml`) to :file:`sdk.yaml`
and place it at the same nesting level:

.. code-block:: console

   $ ls fake-store/sdk-store/my-sdk/latest/edge

     my-sdk.sdk  sdk.yaml


Run the local store
-------------------

Pass the top-level SDK store directory to this :command:`go run` command:

.. code-block:: console

   $ go run github.com/fsouza/fake-gcs-server@latest \
     -data fake-store/ \
     -filesystem-root fake-store/ \
     -scheme http -port 8080 \
     -public-host localhost:8080

     time=1990-01-01T00:00:00.000+00.00 level=INFO msg="server started at http://0.0.0.0:8080"


Use the local store with |project_markup|
-----------------------------------------

To override the URL that |project_markup| uses to connect to SDK Store,
configure the |project_markup| snap
with the address from :option:`!-public-host` in the step above,
adding :samp:`/storage/v1/` as the path:

.. code-block:: console

   $ sudo snap set workshop store.url=http://localhost:8080/storage/v1/
   $ sudo snap restart workshop


|project_markup| will now use the local store.


Revert changes
--------------

To go back to the default store:

.. code-block:: console

   $ sudo snap set workshop store.url=""


|project_markup| will now use the default URL.
