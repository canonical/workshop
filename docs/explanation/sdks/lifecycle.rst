.. _exp_sdk_lifecycle:

.. meta::
   :description: Explanation of the SDK lifecycle in Workshop, walking through
                 sketching, in-project SDKs, standalone SDKs built with SDKcraft,
                 publishing to the SDK Store, and consumption in a workshop.

SDK lifecycle
=============

An SDK rarely springs into existence ready to publish.
It usually starts as a quick local hack,
hardens into a project-private artifact,
and only then graduates into a fully packaged release on the SDK Store.
The shape of the definition stays similar throughout;
what changes is where it lives,
who can see it,
and how |ws_markup| installs it.

The lifecycle has five stages:

#. **Sketch an SDK.**
   This is a throwaway local experiment
   in a single workshop.

#. **Save it as an in-project SDK.**
   The definition moves next to the project's source code
   and is committed to version control.

#. **Build an SDK project.**
   This is a complete |sdk_markup| project
   with parts, hooks, platforms, and tests.

#. **Publish the SDK.**
   Register the name on the SDK Store,
   upload built artifacts,
   and release them to channels.

#. **Consume the SDK.**
   Add the SDK to a :file:`workshop.yaml` definition
   and pick a channel.


Not every SDK travels the whole road,
and the sequence may vary;
for instance, you can create an in-project SDK manually
without a sketch SDK to eject it from.
However, the general sequence is common enough
to be presented as a single flow:


.. mermaid::
   :alt: Flowchart of the SDK lifecycle as five stages, each labeled with the
         commands used at that stage. A sketch SDK (workshop sketch-sdk) is
         ejected (workshop sketch-sdk --eject) into an in-project SDK, which is
         promoted into a standalone SDK project built with sdkcraft init, pack,
         try, and test, then published to the SDK Store with sdkcraft login,
         register, create-track, upload, and release, and finally consumed in a
         workshop with sdk find, workshop launch, and workshop info. An
         in-project SDK can also be authored by hand.
   :caption: SDK lifecycle stages and their commands
   :align: center

   flowchart TD
     subgraph Sketch
       SketchEdit[workshop sketch-sdk]
     end

     subgraph Project[In-project SDK]
       SketchEject[workshop sketch-sdk --eject]
     end

     subgraph Build
       BuildInit[sdkcraft init] --> BuildPack[sdkcraft pack] --> BuildTry[sdkcraft try] --> BuildTest[sdkcraft test]
     end

     subgraph Publish
       PubLogin[sdkcraft login] --> PubRegister[sdkcraft register] --> PubTrack[sdkcraft create-track] --> PubUpload[sdkcraft upload] --> PubRelease[sdkcraft release]
     end

     subgraph Consume
       UseFind[sdk find] --> UseLaunch[workshop launch] --> UseInfo[workshop info]
     end

     SketchEdit --> SketchEject
     SketchEject -->|promote| BuildInit
     BuildTest -->|publish| PubLogin
     PubRelease -->|add to workshop.yaml| UseFind
     Manual[Author by hand] -.-> Project


Each stage trades immediacy for reach.
A sketch is instant but lives and dies with one workshop;
an in-project SDK is shared through the project repository;
a published SDK is packaged once
and consumed anywhere through the SDK Store.
Each command above is documented in the SDK CLI reference,
while the deeper mechanics of each stage,
from hooks and parts to channels and guardrails,
are covered in the SDK concepts and how-to guides.


See also
--------

Explanation:

- :ref:`exp_in_project_sdk`
- :ref:`exp_sdk_best_practices`
- :ref:`exp_sdk_concepts`
- :ref:`exp_sdk_hooks`
- :ref:`exp_sdk_parts`
- :ref:`exp_sdk_store`
- :ref:`exp_sketch_sdk`


How-to guides:

- :ref:`how_build_sdk`
- :ref:`how_publish_sdk`


Reference:

- :ref:`ref_sdk__cli`
- :ref:`ref_sdkcraft__cli`
- :ref:`ref_workshop__cli`


Tutorial:

- :ref:`tut_craft_sdks`
- :ref:`tut_sketch_sdks`
