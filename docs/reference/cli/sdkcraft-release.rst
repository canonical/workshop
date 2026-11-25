.. _ref_sdkcraft_release:


.. meta::
   :description: Reference documentation for the 'sdkcraft release' command

sdkcraft release
----------------

Release an SDK revision to store channels

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft release SDK REVISION CHANNELS

.. rubric:: Description


Release <sdk> at <revision> to the selected store <channels>.
<channels> is a comma-separated list of valid channels on the store.

The <revision> must exist on the store; to see available revisions,
run `sdkcraft revisions <sdk>`.

The channel map is displayed after the operation.

The format for a channel is [<track>/]<risk>[/<branch>], where:

- <track> is used to have long-term release channels.
- <risk> can only be `stable`, `candidate`, `beta`, or `edge`.
- <branch> is optional and dynamically creates a channel with
  a one-month expiration.


.. rubric:: Examples


Release revision 8 to stable:

.. code-block:: console

   $ sdkcraft release my-sdk 8 stable


Release revision 8 to latest/stable:

.. code-block:: console

   $ sdkcraft release my-sdk 8 latest/stable


Release revision 9 to multiple channels:

.. code-block:: console

   $ sdkcraft release my-sdk 9 beta,edge

