.. _exp_sdk_lifecycle:

.. meta::
   :description: Explanation of the SDK lifecycle in Workshop, walking through
                 sketching, in-project SDKs, standalone SDKs built with SDKcraft,
                 publishing to the SDK Store, and consumption in a workshop.

SDK lifecycle
=============

.. @artefact SDK
.. @artefact SDK Store
.. @artefact in-project SDK
.. @artefact sketch SDK
.. @artefact sdkcraft (CLI)

An SDK rarely springs into existence ready to publish.
It usually starts as a quick local hack,
hardens into a project-private artifact,
and only then graduates into a fully packaged release on the SDK Store.
The shape of the definition stays similar throughout;
what changes is where it lives,
who can see it,
and how |ws_markup| installs it.

The lifecycle has five stages:

#. **Sketch.**
   A throw-away local experiment
   inside a single workshop.
#. **In-project SDK.**
   A definition that lives next to the project's source code,
   committed to version control.
#. **Standalone SDK.**
   A full |sdk_markup| project
   with parts, hooks, platforms, and tests.
#. **Publish.**
   Register the name on the SDK Store
   and upload artifacts to channels.
#. **Consume.**
   Add the SDK to a :file:`workshop.yaml` definition
   and pick a channel.


Not every SDK travels the whole road,
and the same SDK can move forward and back between stages.
A published SDK is still useful as a sketch
when you need to try a one-off tweak in a workshop;
a polished standalone project can be temporarily downgraded
to an in-project SDK
while a breaking redesign is in progress.


Sketch SDKs
-----------

A :ref:`sketch SDK <exp_sketch_sdk>` is the lowest-friction entry point.
You run :command:`workshop sketch-sdk` inside a running workshop;
|ws_markup| opens a minimal :file:`sdk.yaml`
with empty :samp:`hooks`, :samp:`plugs`, and :samp:`slots` sections.
When you save and exit,
the workshop refreshes
and applies whatever you wrote.

Sketch SDKs are intentionally limited.
There is exactly one sketch SDK per workshop,
the sketch carries no persistent SDK state across refreshes,
and the definition itself lives only inside that workshop's container.
Once you :command:`workshop remove` the workshop,
the sketch is gone.

This is the right stage when you need to answer questions like:

- Does this tool actually install cleanly under |ws_markup|?
- Which plugs does it need before it works end-to-end?
- What does the workshop look like after the hook runs?


Sketches are not a publishable artifact
and aren't meant to be.
They earn their keep
by letting you iterate on hook logic
in seconds rather than minutes,
without involving |sdk_markup|, parts, or platforms at all.
When the sketch behaves the way you want,
the natural next move is to promote it.


In-project SDKs
---------------

An :ref:`in-project SDK <exp_in_project_sdk>`
moves the definition out of the workshop
and into the project's :file:`.workshop/` directory.
You get there by running :command:`workshop sketch-sdk --eject`,
which copies the sketch's :file:`sdk.yaml` and hooks
to :file:`.workshop/<NAME>/`
and removes the sketch from the workshop.

From this point on,
the SDK is just another tracked part of your project:

- It lives in git alongside the source code it supports.
- Collaborators get the same SDK automatically
  by cloning the project and refreshing their workshop.
- The workshop definition references the SDK
  with the :samp:`project-` prefix
  (for example, :samp:`project-console`)
  so |ws_markup| knows to load it from :file:`.workshop/`
  rather than the SDK Store.


In-project SDKs are the right stop
when an SDK is genuinely project-specific:
it encodes a private convention,
glues internal services together,
or wraps an in-house tool
that has no audience outside the team.
There is no upload step,
no name registration,
and no channel discipline to enforce;
in exchange,
discovery is limited to whoever already has the project.

When the same in-project SDK ends up being copied
into a second project, then a third,
that is usually a signal that it has outgrown this stage.


Standalone SDKs
---------------

A standalone SDK is what most people mean
when they say "an SDK".
It is a |sdk_markup| project on its own:
its own repository,
its own :file:`sdkcraft.yaml`,
its own :file:`hooks/` tree,
and its own CI.
All this may come from an in-project SDK that got promoted,
or it may be created using the
`canonical/template-sdk <https://github.com/canonical/template-sdk>`__
repository.

The definition declares :samp:`name`, :samp:`version`,
:samp:`summary`, :samp:`description`, :samp:`license`,
and the set of :samp:`platforms` |sdk_markup| should build for.
:ref:`Parts <exp_sdk_parts>` describe how to obtain
and arrange the SDK's payload.
:ref:`Plugs and slots <exp_interfaces>` declare
the host resources the SDK needs
and the in-workshop resources it provides.
:ref:`Hooks <exp_sdk_hooks>` carry the run-time logic:
:samp:`setup-base`, :samp:`setup-project`,
:samp:`save-state`, :samp:`restore-state`, and :samp:`check-health`.

Two |sdk_markup| commands tie this stage together.
:command:`sdkcraft pack` produces a :file:`.sdk` artifact
named after the SDK, its architecture, and its base,
for example :file:`ollama_amd64_ubuntu@24.04.sdk`.
:command:`sdkcraft try` packs the SDK
and then drops it into a *try area* that |ws_markup| reads from
when you reference it as :samp:`try-<NAME>` in a workshop;
this is how you exercise an SDK end-to-end
before the Store ever sees it.

What's gained at this stage is not the ability to ship
(in-project SDKs can already be shared via git)
but consistency:
the SDK can be built in CI,
versioned on its own track,
and tested against a clean container
rather than against whatever your laptop happens to have installed.


Publishing
----------

Publishing turns a packed standalone SDK
into something other |ws_markup| users can pull from the SDK Store.
The SDK Store is the distribution channel:
named SDKs,
versioned tracks and risks,
and authenticated uploads.

Three commands shape the publishing flow:

- :command:`sdkcraft register <NAME>` reserves an SDK name
  on the SDK Store.
  This runs once per SDK, ever.

- :command:`sdkcraft upload <FILE.sdk>` pushes a built artifact
  and returns a revision number.
  Passing :option:`!--release` releases that revision
  to one or more channels in the same step.

- :command:`sdkcraft release <NAME> <REVISION> <CHANNELS>`
  promotes an existing revision to additional channels later,
  without rebuilding or re-uploading.


Channels follow the same :samp:`[<TRACK>/]<RISK>[/<BRANCH>]` shape
as the rest of the Canonical ecosystem,
for example :samp:`latest/stable`, :samp:`1.x/edge`,
or a plain :samp:`stable` when no track is specified.
A track usually groups revisions
that share a major version;
a risk level signals how production-ready the revision is.
The track is optional;
omitting it targets the default :samp:`latest` track.

Publishing has no local-only or dry-run mode.
Register, upload, and release all talk to the live SDK Store,
which means an authenticated account
and a stable network connection
are prerequisites.
This is also why the publish stage is rarely the place
to discover problems in the SDK itself;
the iteration loops earlier in the lifecycle
(sketch, try, test)
exist precisely so that the published artifact
is the boring step.


Consumption
-----------

Once an SDK exists on the Store,
consuming it is a single entry in :file:`workshop.yaml`:

.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: latest/stable


|ws_markup| resolves the SDK against the Store,
verifies that one of its supported :samp:`platforms`
matches the workshop's :samp:`base`,
and installs it during :command:`workshop launch`.
After that the SDK behaves the same way
as a sketch or an in-project SDK would:
hooks run at the appropriate lifecycle stages,
plugs and slots negotiate access to host resources,
and :command:`workshop info` reports the SDK alongside the others.

The consumer also picks how aggressively to follow updates.
A workshop pinned to :samp:`latest/stable`
sees a new SDK revision only when its publisher releases one to that channel;
a workshop tracking :samp:`latest/edge`
will pick up changes as soon as the publisher uploads them.
This is the same trade-off
that applies to any channel-based distribution mechanism,
and it is one of the reasons
SDKs are published at all
rather than copied between projects:
consumers get a predictable upgrade story
without coordinating directly with the publisher.


Tooling for authors
-------------------

New SDKs should start from
`canonical/template-sdk <https://github.com/canonical/template-sdk>`__,
a GitHub-template repository
that ships the :file:`sdkcraft.yaml` skeleton,
a :file:`hooks/` tree,
a :file:`VERSION` file pinning the upstream release,
a :file:`renovate.json` that tracks upstream releases on a long-lived branch,
and CI workflows that build on pull requests
and upload to the SDK Store on push to that branch.
The template makes the renovate-and-upload loop the default path:
once an SDK is registered on the Store,
upstream releases land as automated revisions
without further manual work.

Inside the template,
the :samp:`sdk-designer` agentic skill
runs an interactive scaffolding conversation,
asking about the software to package
and filling in the template files.
Authors who don't use the skill
edit the template files by hand;
the result is the same shape either way.


See also
--------

Explanation:

- :ref:`exp_in_project_sdk`
- :ref:`exp_interfaces`
- :ref:`exp_sdk_best_practices`
- :ref:`exp_sdk_concepts`
- :ref:`exp_sdk_hooks`
- :ref:`exp_sdk_parts`
- :ref:`exp_sketch_sdk`


How-to guides:

- :ref:`how_build_sdk`
- :ref:`how_publish_sdk`


Tutorial:

- :ref:`tut_craft_sdks`
- :ref:`tut_sketch_sdks`
