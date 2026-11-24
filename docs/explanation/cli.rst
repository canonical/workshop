.. _exp_cli:

.. meta::
   :description: Overview of the four Workshop command-line tools (workshop,
                 sdk, sdkcraft, and workshopctl) and the command categories
                 each one provides.

Command-line tools
==================

|ws_markup| works through four command-line tools,
each aimed at a different audience and stage of the workflow.
:program:`workshop` and :program:`sdk`
are the everyday tools for people working in workshops.
:program:`sdkcraft`, shipped as its own snap,
is the SDK-author tool that publishers run on their workstation.
:program:`workshopctl` is an in-workshop helper
that SDK hooks invoke to report state back to the daemon.


.. _exp_workshop_cli:

workshop
--------

:program:`workshop` is the daily go-to instrument for regular users,
with a set of commands that govern the entire lifecycle of a
:ref:`workshop <exp_workshop>`.

There are several categories of commands that vary by their purpose:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 10 11 20

   * - Actions
     - Commands
     - What they do

   * - Create, update, delete
     - :command:`init`,
       :command:`launch`,
       :command:`refresh`,
       :command:`remove`,
       :command:`restore`,
       :command:`start`,
       :command:`stop`
     - Create a workshop definition,
       then govern the workshop's existence and runtime state,
       from first launch to refresh, restore, and removal.

   * - Customize
     - :command:`sketch-sdk`,
       :command:`sketches`
     - Augment a workshop with project-specific customizations
       through sketch SDKs.

   * - Enumerate
     - :command:`info`,
       :command:`list`
     - List the workshops in a project and inspect their current details.

   * - Track changes
     - :command:`changes`,
       :command:`tasks`
     - Review recent changes to the workshops in a project
       and the tasks that make up each change.

   * - Manage connections
     - :command:`connect`,
       :command:`connections`,
       :command:`disconnect`,
       :command:`remount`
     - Wire interface plugs and slots between SDKs,
       list existing connections, and remount their sources.

   * - Run shell commands
     - :command:`exec`,
       :command:`shell`
     - Run an ad-hoc command in a workshop
       or open an interactive shell inside it.

   * - Run named actions
     - :command:`actions`,
       :command:`run`
     - List and invoke the named actions
       defined in a workshop's :samp:`actions:` section.

   * - Manage warnings
     - :command:`okay`,
       :command:`warnings`
     - List warnings raised by the daemon and acknowledge them.


.. note::

   :program:`workshop` talks to the |ws_markup| daemon,
   :program:`workshopd`, via a REST API,
   so alternatives are possible and, in fact, encouraged.


.. _exp_sdk_cli:

sdk
---

:program:`sdk` makes it easy to find and learn more about
the SDKs available to you.
Like :program:`workshop`,
it talks to the |ws_markup| daemon,
:program:`workshopd`, over a REST API.

There is one category of commands:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 10 11 20

   * - Actions
     - Commands
     - What they do

   * - Discover
     - :command:`find`,
       :command:`info`,
       :command:`list`
     - Search the SDK Store,
       enumerate the SDKs available on your machine,
       and inspect their details.


.. _exp_sdkcraft_cli:

sdkcraft
--------

:program:`sdkcraft` is what an SDK publisher uses on their workstation
to scaffold, build, test, try, and publish SDKs to the SDK Store.

There are several categories of commands that vary by their purpose:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 10 11 20

   * - Actions
     - Commands
     - What they do

   * - Lifecycle
     - :command:`build`,
       :command:`clean`,
       :command:`pack`,
       :command:`prime`,
       :command:`pull`,
       :command:`stage`,
       :command:`test`,
       :command:`try`
     - Work through the parts-based build pipeline
       to produce an SDK artefact
       and try it locally before publishing.

   * - Store
     - :command:`create-track`,
       :command:`register`,
       :command:`release`,
       :command:`revisions`,
       :command:`upload`
     - Claim an SDK name, manage its tracks, upload artefacts,
       and list or release revisions to channels.

   * - Store account
     - :command:`login`,
       :command:`whoami`
     - Authenticate against the SDK Store
       and inspect the active identity.

   * - Other
     - :command:`init`,
       :command:`version`
     - Bootstrap a new project layout
       and report the installed |sdk_markup| version.


.. note::

   |sdk_markup| is a separate snap, installed independently of |ws_markup|.
   See the `SDKcraft`_ repository for installation and release notes.


.. _exp_workshopctl_cli:

workshopctl
-----------

:program:`workshopctl` reports SDK state back to the |ws_markup| daemon
over a restricted socket.
:ref:`SDK hooks <exp_sdk_hooks>` invoke it from inside a running workshop;
it is not intended for end users to call directly.

There is one category of commands:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 10 11 20

   * - Actions
     - Commands
     - What they do

   * - Report SDK health
     - :command:`set-health`
     - Let the daemon know whether the SDK is :samp:`okay`,
       :samp:`waiting`, or in an :samp:`error` state,
       with an optional machine-readable error code
       and a human-readable message.


.. note::

   :program:`workshopctl` only works from an SDK hook context,
   where the daemon supplies a context cookie via the
   :envvar:`WORKSHOP_COOKIE` environment variable.
   Running it from an interactive shell returns
   ``cannot invoke workshopctl operation commands ... from outside of a workshop``.


See also
--------

Reference:

- :ref:`ref_cli`
- :ref:`ref_sdk_hooks`


Tutorial:

- :ref:`tut_get_started`
