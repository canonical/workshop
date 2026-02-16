:slug: home-page
:relatedlinks: [Workshop](https://github.com/canonical/workshop/), [SDKcraft](https://github.com/canonical/sdkcraft/), [LXD](https://documentation.ubuntu.com/lxd/), [Snap](https://snapcraft.io/docs/)

.. _home:

.. meta::
   :description: Home page for Workshop documentation, providing links to
                 tutorials, how-to guides, references, and explanations.

|ws_markup|
===========

.. toctree::
   :hidden:

   Home <self>
   tutorial/index
   how-to/index
   reference/index
   explanation/index
   Release notes <release-notes/index>
   sdks/index
   Contribution <contributing>


**A tool for defining and handling ephemeral development environments**.

**List your dependencies and components in YAML to define an environment**.
The key pieces of a definition are SDKs,
independent but connectable units of functionality
created by software publishers and available on the SDK Store.
|ws_markup| simplifies experiments with your environment layout.

**It allows you to focus on adding value to your project**.
With |ws_markup|, you can launch a setup
that previously took hours to configure in a few commands,
and be sure that it stays operational.
It assists in issue reproduction,
enables hands-on code reviews,
and turns environment updates into manageable transactions,
reducing the need to battle with your tooling every day.

**For those who build and maintain complex, error-prone workspaces**.
AI/ML, robotics, IoT, EdTech, and similar domains
typically use less-than-trivial project layouts
that depend on multiple Linux distributions or images,
a plethora of SDKs from many vendors,
and a grocery list of libraries and languages.
That's where |ws_markup| thrives.

----



In this documentation
---------------------

.. list-table::
   :widths: 20 80
   :class: borderless

   * - **Tutorial**
     - :ref:`Get started <tut_get_started>` •
       :ref:`Work with interfaces <tut_interfaces>` •
       :ref:`Sketch SDKs <tut_sketch_sdks>` •
       :ref:`Craft SDKs <tut_craft_sdks>`

   * - **Workshops**
     - :ref:`Concepts <exp_workshop_concepts>` •
       :ref:`Launch <ref_workshop_launch>` •
       :ref:`Refresh <ref_workshop_refresh>` •
       :ref:`Connect <ref_workshop_connect>` •
       :ref:`Shell access <ref_workshop_shell>` •
       :ref:`Add actions <how_add_actions>` •
       :ref:`Status diagrams <ref_workshop_status>` •
       :ref:`Definition file <ref_workshop_definition>`

   * - **SDKs**
     - :ref:`Concepts <exp_sdk_concepts>` •
       :ref:`Sketch SDKs in-place <tut_sketch_sdks>` •
       :ref:`Craft full SDKs <tut_craft_sdks>` •
       :ref:`Design best practices <exp_sdk_best_practices>` •
       :ref:`SDKs vs Dockerfiles <exp_dockerfile_vs_sdk>` •
       :ref:`Definition file <ref_sdk_definition>` •
       :ref:`Internals <ref_sdk_internals>`

   * - **Interfaces**
     - :ref:`Concepts <exp_interface_concepts>` •
       :ref:`Camera <exp_camera_interface>` •
       :ref:`Desktop <exp_desktop_interface>` •
       :ref:`GPU <exp_gpu_interface>` •
       :ref:`Mounts <exp_mount_interface>` •
       :ref:`SSH agent <exp_ssh_interface>` •
       :ref:`Networking <exp_tunnel_interface>`

   * - **Development**
     - :ref:`Connect VS Code <how_vscode_connect_remote>` •
       :ref:`JetBrains Gateway <how_jetbrains_gateway>` •
       :ref:`VS Code in browser <how_vscode_run_in_browser>` •
       :ref:`JupyterLab in browser <how_jupyterlab_run_in_browser>` •
       :ref:`Use with Git <how_git_workshops>` •
       :ref:`GitHub Actions locally <how_run_github_actions_locally>`

   * - **Projects**
     - :ref:`Concepts <exp_projects>` •
       :ref:`Move projects <how_move_projects>` •
       :ref:`Update projects <tut_project_updates>` •
       :ref:`Changes and tasks <exp_changes_tasks>`

   * - **Troubleshooting**
     - :ref:`Debug workshops <how_debug_issues_workshops>` •
       :ref:`Fix installation <how_troubleshoot>` •
       :ref:`Resolve plug conflicts <how_resolve_plug_conflicts>` •
       :ref:`Purge workshops <how_purge>`

   * - **Reference**
     - :ref:`Workshop CLI <ref_workshop__cli>` •
       :ref:`SDKcraft CLI <ref_sdkcraft__cli>` •
       :ref:`Reference SDKs <ref_sdks_index>` •
       :ref:`Workshop internals <ref_workshop_internals>` •
       :ref:`SDK internals <ref_sdk_internals>` •
       :ref:`Runtime behavior <exp_arch_runtime_behavior>`


How this documentation is organised
------------------------------------

This documentation follows the `Diátaxis documentation framework <https://diataxis.fr/>`_,
organizing content by the type of information users need.

:doc:`Tutorial <tutorial/index>`: Hands-on learning path for new |ws_markup| users,
progressing from basic operations through interface usage to SDK development.

:doc:`How-to guides <how-to/index>`: Step-by-step instructions for specific tasks
like connecting IDEs, managing projects, and troubleshooting issues.

:doc:`Reference <reference/index>`: Technical specifications for CLI commands,
definition file formats, and internal behavior.

:doc:`Explanation <explanation/index>`: In-depth discussion of |ws_markup| architecture,
concepts, and design principles.

----

.. _project_community:

Project and community
---------------------

|ws_markup| is an emergent project
within the DevEx department here at Canonical;
|sdk_markup| is its sibling project,
aimed at publishers who create and distribute SDKs for |ws_markup|.

At its core, |ws_markup| builds upon Canonical's mature tech.
It uses `LXD`_ as the underlying container technology;
it also follows the tooling paradigm exemplified by
`Snap <https://snapcraft.io/docs/>`_,
and implemented with
`Craft CLI <https://craft-cli.readthedocs.io/>`_.

Talk to us if you have a project in AI/ML, robotics, or any other field
where setting up an environment is a daily or weekly activity
that can easily shave hours off your schedule.
Tell us about the frustrating parts of your journey,
and we'll see what |ws_markup| can do for you.
Let us know if you have an SDK or framework to try with |ws_markup|:
we'll help you get it out there.

.. rubric:: Get involved

- `Pulse reviews on Discourse <https://discourse.canonical.com/c/engineering/sdk/34>`__
- `Mattermost channel <https://chat.canonical.com/canonical/channels/sdk>`__
- :ref:`Contribution and participation <contributing>`

.. rubric:: Releases and roadmap

- :ref:`Release notes <release_notes>`
- `Product map <https://warthogs.atlassian.net/jira/software/c/projects/WSP/boards/1645>`__

.. rubric:: Governance and policies

- `Code of conduct <https://ubuntu.com/community/ethos/code-of-conduct>`__

.. rubric:: Feedback and support

- `Product and documentation feedback <https://github.com/canonical/workshop/issues>`__
