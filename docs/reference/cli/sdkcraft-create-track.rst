.. _ref_sdkcraft_create_track:


.. meta::
   :description: Reference documentation for the 'sdkcraft create-track' command

sdkcraft create-track
---------------------

.. @artefact sdkcraft create-track

Create one or more tracks for an SDK on the SDK Store

.. rubric:: Usage

.. code-block:: console

   $ sdkcraft create-track --track TRACKS SDK

.. rubric:: Description


Create one or more tracks for an SDK on the SDK Store.

The command lists all tracks it created.
Tracks must match an existing guardrail for this SDK.


.. rubric:: Flags


--track

   The track name to create (can be repeated)


.. rubric:: Examples


Create two tracks for the "go" SDK:

.. code-block:: console

   $ sdkcraft create-track --track 1.27 --track 1.25 go

