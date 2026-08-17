.. _how_publish_sdk:

.. meta::
   :description: How-to guide for publishing a Workshop SDK to the SDK Store with
                 SDKcraft, covering pack, test, try, register, upload, and release,
                 plus how to consume the published SDK from a workshop definition.

How to publish an SDK
=====================

.. @tests in tests/docs-how-to/publish-an-sdk/task.yaml

.. @artefact SDK
.. @artefact SDK Store
.. @artefact sdkcraft (CLI)
.. @artefact try SDK

Publishing turns a packed SDK
into something other |ws_markup| users can pull from the SDK Store.
If the SDK isn't yet packed, tested, and tried locally,
go through :ref:`how_build_sdk` first.

The publishing flow has four steps:

#. **Pack** the SDK into one :file:`.sdk` artifact per platform.
#. **Register** the SDK name on the SDK Store.
#. **Upload** a revision.
#. **Release** the revision to one or more channels.


The first step runs on your machine.
The last three talk to the live SDK Store
at :samp:`api.charmhub.io`
and require an authenticated account.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- Complete :ref:`how_build_sdk`
  so that the SDK is fully built, tested, and tried locally.

- |sdk_markup| installed.

- LXD 6.8 or later running on the host.

- An Ubuntu One account.

- The SDK source tree is clean and ready to build.


There is no local-only or dry-run mode for the Store-side commands.
Plan to publish from a workstation with a stable network connection.


Pack the SDK
------------

:command:`sdkcraft pack` builds the SDK and packs it into one artifact
per platform declared in :file:`sdkcraft.yaml`:

.. code-block:: console

   $ sdkcraft pack


The resulting filenames follow the pattern
:file:`<NAME>_<ARCH>_<BASE>.sdk`,
for example :file:`<NAME>_amd64_ubuntu@24.04.sdk`.
:command:`sdkcraft pack` differs from :command:`sdkcraft try`
in one respect:
the artifacts stay in the working directory
rather than being copied into the :ref:`try area <exp_test_try_sdk>`.

If a previous build left state behind,
clean and rebuild from scratch:

.. code-block:: console

   $ sdkcraft clean && sdkcraft pack


Register the SDK name
---------------------

Each SDK on the Store has a unique name.
Reserve yours once per SDK, ever.

Any authenticated Ubuntu One account can register an available name
and publish under it,
much as anyone can register a new snap name.
Once a name is registered,
only the SDK's publisher and collaborators
can upload or release revisions to it.

Authenticate first:

.. code-block:: console

   $ sdkcraft login


Confirm the right account is active:

.. code-block:: console

   $ sdkcraft whoami


Then register the SDK name:

.. code-block:: console

   $ sdkcraft register <NAME>


Names are global to the SDK Store
and normally cannot be re-registered after release.
Pick a name that matches the SDK's :samp:`name` field in :file:`sdkcraft.yaml`
and that you intend to keep.


Upload a revision
-----------------

Each :command:`sdkcraft upload` invocation pushes one :file:`.sdk` file
and assigns it a revision number on the Store:

.. code-block:: console

   $ sdkcraft upload <NAME>_amd64_ubuntu@24.04.sdk


The output reports the revision number.
At this point, the revision is on the Store
but isn't released to any channel yet,
so :command:`sdk find` won't return it.

To upload and release in one step,
pass :option:`!--release` with one or more channels:

.. code-block:: console

   $ sdkcraft upload <NAME>_amd64_ubuntu@24.04.sdk --release latest/edge


Upload one artifact per platform.
If :command:`sdkcraft pack` produced
:file:`<NAME>_amd64_ubuntu@22.04.sdk` and
:file:`<NAME>_amd64_ubuntu@24.04.sdk`,
upload both;
the Store tracks revisions per platform.


.. _how_publish_sdk_ci:

Automate uploads from CI
------------------------

The :file:`.github/workflows/` files that ship with the
`canonical/template-sdk <https://github.com/canonical/template-sdk>`_
repository
run :command:`sdkcraft upload --release` automatically
on push to the version branch that :file:`renovate.json` maintains.
After the one-time :command:`sdkcraft register`,
upstream releases land as automated revisions
without further manual commands:
Renovate opens a PR bumping :file:`VERSION`,
the merge of that PR triggers the upload workflow,
and the new revision shows up in the configured channels.

The workflow runs non-interactively,
so it authenticates to the SDK Store
from credentials passed in the
:envvar:`SDKCRAFT_STORE_CREDENTIALS` environment variable.
The template's :file:`.github/workflows/upload.yml`
feeds that variable from a repository secret.
Create that secret once.

Export a set of credentials to a file with :option:`!--export`.
This logs in without touching the local keyring
and writes the credentials to the named file:

.. code-block:: console

   $ sdkcraft login --export credentials.txt


The contents of :file:`credentials.txt`
are the value to store as the repository secret.


In your SDK repository on GitHub,
navigate to
:guilabel:`Settings` > :guilabel:`Secrets and variables` > :guilabel:`Actions`,
select :guilabel:`New repository secret`,
and add the credentials under the name :samp:`SDKCRAFT_STORE_CREDENTIALS_PROD`.

The template ships :file:`.github/workflows/upload.yml`
pointed at a staging secret,
so a freshly created SDK doesn't publish
to the production Store before it's ready.
To publish for real,
point the :samp:`secrets:` mapping at the secret you just created:

.. code-block:: yaml
   :caption: .github/workflows/upload.yml

   secrets:
     SDKCRAFT_STORE_CREDENTIALS: ${{ secrets.SDKCRAFT_STORE_CREDENTIALS_PROD }}


For what else the template ships,
see :ref:`how_build_sdk`.


Release a revision
------------------

When a revision is on the Store but not yet released,
or when promoting an existing revision
from one channel to another,
use :command:`sdkcraft release`:

.. code-block:: console

   $ sdkcraft release <NAME> <REVISION> <CHANNELS>


Channels follow the :samp:`[<TRACK>/]<RISK>[/<BRANCH>]` shape:

- :samp:`<TRACK>` is optional and groups related revisions,
  typically along major-version lines or variations in supported platforms
  (for example, :samp:`1.x` or :samp:`nvidia`).
  Omitting it targets the default :samp:`latest` track.

  .. caution::
     Do not use the base (for example, :samp:`24.04`)
     as the track name.
     The SDK Store tracks revisions per platform automatically
     (see the platforms listed in :file:`sdkcraft.yaml`),
     so a per-base track only duplicates that
     and limits how revisions can be grouped meaningfully.

- :samp:`<RISK>` is one of
  :samp:`stable`, :samp:`candidate`, :samp:`beta`, or :samp:`edge`.

- :samp:`<BRANCH>` is optional
  and creates a short-lived channel with a one-month expiration.


Plain :samp:`stable` and comma-separated lists like :samp:`beta,edge`
are valid channel arguments.

For example,
to promote revision 8 to :samp:`latest/stable`:

.. code-block:: console

   $ sdkcraft release <NAME> 8 latest/stable


:command:`sdkcraft release` is idempotent
and never rebuilds or re-uploads;
it only adjusts the channel map.


Release to a non-default track
------------------------------

Releasing to :samp:`latest` needs no setup;
the :samp:`latest` track always exists.
Releasing to any other track requires that the track exist first.
The :command:`sdkcraft create-track` command creates one:

.. code-block:: console

   $ sdkcraft create-track <NAME> --track 1.x


It only creates track names
that the SDK Store already permits through a *guardrail* for your SDK,
a Store-side pattern such as one matching :samp:`1.x`-style version tracks.
Without a matching guardrail, :command:`sdkcraft create-track` is rejected.

Guardrails are not self-service.
To request one,
open a `GitHub issue <https://github.com/canonical/workshop/issues>`_
on the |ws_markup| repository,
naming the SDK and the track pattern you need
(for example, version tracks like :samp:`1.x` or :samp:`2.x`).
The |ws_markup| team triages the request
and coordinates track creation with the SDK Store team.
For the level of detail a request should carry,
see how the wider ecosystem handles
`snap track and guardrail requests <https://forum.snapcraft.io/t/create-new-track-and-guardrails-for-registry-snap/51209>`_.


Consume the published SDK
-------------------------

Once a revision is released to a channel,
any |ws_markup| user can pull it
by referencing the SDK in :file:`workshop.yaml`:

.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: <NAME>
       channel: latest/stable


The workshop's :samp:`base`
must match one of the SDK's supported platforms.


See also
--------

Explanation:

- :ref:`exp_sdk_concepts`
- :ref:`exp_test_try_sdk`


How-to guides:

- :ref:`how_build_sdk`


Reference:

- :ref:`ref_sdkcraft_create_track`
- :ref:`ref_sdkcraft_login`
- :ref:`ref_sdkcraft_pack`
- :ref:`ref_sdkcraft_register`
- :ref:`ref_sdkcraft_release`
- :ref:`ref_sdkcraft_test`
- :ref:`ref_sdkcraft_try`
- :ref:`ref_sdkcraft_upload`
- :ref:`ref_workshop_definition`


Tutorial:

- :ref:`tut_craft_sdks`
