.. _exp_sdk_concepts:

.. meta::
   :description: A comprehensive overview of SDK concepts in Workshop, explaining
                 what SDKs are, how they install packages and configure environments,
                 and how they're distributed through an SDK Store with channel-based versioning.

SDK concepts
============

.. @artefact SDK
.. @artefact SDK publisher
.. @artefact SDK Store

With |sdk_markup|, you can package and publish software dependencies
as isolated *SDKs* to be used in a workshop definition by |ws_markup|,
instead of managing them system-wide or through container images.
SDKs encapsulate all required functionality,
keeping installations clean and limiting access to system-level capabilities.
Publishers handle installation and updates for SDKs,
freeing users from maintaining complex image definitions or configurations.

Most SDKs are designed by publishers
and made available via the SDK Store,
but some are specific to a particular project or user.
A single workshop can include multiple SDKs from different sources.
SDKs are distributed through channels similar to
`snap channels <https://snapcraft.io/docs/channels/>`_.


.. _exp_sdk_definition:

SDK definition
--------------

.. @artefact SDK definition

An SDK is defined by the SDK publisher;
the definition may look like this:

.. @artefact sdkcraft (CLI)

.. literalinclude:: ../../examples/go-sdkcraft.yaml
   :language: yaml
   :caption: sdkcraft.yaml


Lifecycle hooks
---------------

.. @artefact SDK hook

|ws_markup| and |sdk_markup| enable optional lifecycle hooks
that let an SDK extend the workshop's internal behavior.
Hooks live in the :file:`hooks/` subdirectory next to the SDK definition
and run as bash scripts at well-defined points
during launch and refresh.
See :ref:`exp_sdk_hooks` for the contract,
the per-hook environment,
and how the SDK can talk back to the daemon with :program:`workshopctl`.
State that has to outlive an SDK refresh
is handled by the :samp:`save-state` and :samp:`restore-state` hooks
described in :ref:`exp_sdk_state`.


SDK platforms
-------------

.. @artefact SDK platforms

Platforms describe where SDKs can be built and installed.
Some SDKs include compiled code,
which only certain families of CPUs will understand.
Others depend on particular versions of software provided by the workshop's base image.

The :samp:`platforms` section of the :ref:`definition <exp_sdk_definition>`
lists the platforms that the SDK supports.
Each build corresponds to one of these platforms.
By default, |sdk_markup| builds SDKs for every possible platform.
This typically means all platforms
with the same CPU architecture as the build machine.

When installing an SDK,
|ws_markup| will check its platform metadata for compatibility.

|ws_markup| and |sdk_markup| follow `Debian's naming scheme <https://www.debian.org/ports/>`_
for CPU architectures.
SDKs that don't ship compiled binaries
can use the :samp:`all` architecture instead.


.. _exp_system_sdk:

System SDK
----------

.. @artefact system SDK

Every workshop contains a special *system SDK*
that exposes system resources through its slots.
It cannot be installed from the SDK Store.
Instead, it is automatically installed first during :command:`workshop launch`
and removed last during :command:`workshop remove`
to ensure internal consistency.

The purpose of the system SDK isn't to add hooks or additional content;
it's only there to uniformly expose host system resources to other SDKs.
As such, it can't be removed by the user.
It's also the only SDK
that can have :ref:`mount interface <exp_mount_interface>` slots on the host.

The uniformity of this approach lies in the fact that system resources
and workshop resources are exposed using the same logic.
You can also define additional plugs and slots for the system SDK,
just as with any other SDK.

.. _exp_sketch_sdk:

Sketch SDK
----------

.. @artefact sketch SDK

The sketch SDK is another special type of SDK.
Like the system SDK, it's unavailable from the SDK Store;
instead, you define it inside the workshop
using the :command:`workshop sketch-sdk` command.
Its purpose is to allow |ws_markup| users
to quickly make changes to a workshop
beside the regular SDKs listed in the :ref:`definition <exp_sdk_definition>`.

Unlike a regular SDK, the sketch SDK:

- doesn't carry any persistent data
- doesn't appear on the definition
- is unique to the workshop where it was created


The sketch SDK can have :ref:`hooks <exp_sdk_hooks>`
and use :ref:`interfaces <exp_interface_concepts>`,
which allows it to interact with other SDKs.
Note that :samp:`sketch` is a reserved name,
and the sketch SDK is always installed last.


.. _exp_test_try_sdk:

Testing and trying SDKs
-----------------------

.. @artefact sdkcraft (CLI)
.. @artefact try SDK


Once an SDK is packed,
publishers have two ways to exercise it before upload.

The :command:`sdkcraft test` command runs the SDK's
`spread <https://github.com/canonical/spread>`__ tests
against a freshly packed SDK.
These tests live under the SDK's :file:`tests/` tree
and are the publisher's responsibility to author and maintain;
|sdk_markup| only invokes the spread harness and reports results.

The :command:`sdkcraft try` command allows publishers to test SDKs
before uploading them to the Store.
Once installed in a workshop,
these SDKs behave identically to SDKs from the Store.

|sdk_markup| does not install SDKs in a workshop directly;
it simply copies packed SDKs to a directory called the *try area*.
|ws_markup| looks in this directory
when installing an SDK with the :samp:`try-` prefix.

The try area has no channels;
only one version of an SDK can be tested at a time.
However, this version can be tested in multiple workshops with different :ref:`bases <exp_base>`.


.. _exp_in_project_sdk:

In-project SDKs
---------------

.. @artefact in-project SDK

An *in-project SDK* resides within your project's :file:`.workshop/` directory.
Unlike regular SDKs, which are published and distributed through the SDK Store,
in-project SDKs are specific to your project
and are version-controlled alongside your project's source code.

You can create an in-project SDK by ejecting a :ref:`sketch SDK <exp_sketch_sdk>`
or by adding one manually,
creating the appropriate directory structure with :file:`sdk.yaml` and hooks.
This approach allows you to customize the workshop
to fit your project's unique requirements,
ensuring that all collaborators use the same environment and dependencies.

They are a good fit when your SDK includes project-specific dependencies,
tools, interface connections, or configurations
that should remain private to the project
and not be published or reused elsewhere.


See also
--------

Explanation:

- :ref:`exp_interfaces`
- :ref:`exp_projects`
- :ref:`exp_workshop`


Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_sdk_hooks`
- :ref:`ref_sdk_internals`
- :ref:`ref_workshop_changes`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
- :ref:`ref_workshop_start`
- :ref:`ref_workshop_tasks`
- :ref:`ref_workshopctl__cli`
