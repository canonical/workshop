:slug: home-page
:relatedlinks: [Workshop](https://github.com/canonical/workshop/), [SDKcraft](https://github.com/canonical/sdkcraft/), [LXD](https://canonical.com/lxd/docs/default/), [Snap](https://snapcraft.io/docs/)

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

.. toctree::
   :hidden:

   Release notes <release-notes/index>
   Contribute <contributing>
   Security <security>



**Workshops are secure, fast, and composable development environments
that come agent-ready**.

**Wrap complex, error-prone workspaces
into reliable and reproducible definitions of languages, libraries, and tooling**.
The key pieces of a definition are SDKs:
independent, connectable units of functionality
that publishers package and share on the SDK Store,
and teams can define in their repositories.

**Workshops enable sandboxed experimentation,
turn environment updates into manageable transactions,
and ensure consistent, reproducible environments**.
With |ws_markup|, you can launch a setup
that previously took hours to configure in a few commands
and be sure it will work the same way every time,
or tear it down and start from the last step without worrying about leftover state.

**Agentic engineering, AI/ML, robotics, IoT, EdTech, and similar domains**
typically use less-than-trivial project layouts
that rely on many Ubuntu versions or container images,
a plethora of diverse tools and frameworks,
and a wide range of libraries and languages.
That's where |ws_markup| thrives.

**Built for AI workflows**.
|ws_markup| publishes :ref:`LLM-readable docs <ref_ai_discovery>`,
and ships agentic skills for :ref:`operating workshops <ref_ai_use_workshop_skill>`
and :ref:`scaffolding SDKs <ref_ai_sdk_designer_skill>`.

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
       :ref:`Multi-workshop patterns <exp_multi_workshop_patterns>` •
       :ref:`Use multiple workshops <how_use_multiple_workshops>` •
       :ref:`Status diagrams <ref_workshop_status>` •
       :ref:`Definition <ref_workshop_definition>`

   * - **Interfaces**
     - :ref:`Concepts <exp_interface_concepts>` •
       :ref:`Plugs and slots <exp_plugs_slots>` •
       :ref:`Camera <exp_camera_interface>` •
       :ref:`Custom device <exp_custom_device_interface>` •
       :ref:`Desktop <exp_desktop_interface>` •
       :ref:`GPU <exp_gpu_interface>` •
       :ref:`Mounts <exp_mount_interface>` •
       :ref:`SSH agent <exp_ssh_interface>` •
       :ref:`Networking <exp_tunnel_interface>` •
       :ref:`Add mounts <how_add_mounts>` •
       :ref:`Use host devices <how_use_host_devices>` •
       :ref:`Forward ports <how_forward_ports>`

   * - **SDKs**
     - :ref:`Concepts <exp_sdk_concepts>` •
       :ref:`Lifecycle <exp_sdk_lifecycle>` •
       :ref:`Parts <exp_sdk_parts>` •
       :ref:`Runtime hooks <exp_sdk_hooks>` •
       :ref:`SDKs vs Dockerfiles <exp_dockerfile_vs_sdk>` •
       :ref:`Definition <ref_sdk_definition>`

   * - **Projects**
     - :ref:`Concepts <exp_projects>` •
       :ref:`Move projects <how_move_projects>` •
       :ref:`Update projects <tut_project_updates>` •
       :ref:`Changes and tasks <exp_changes_tasks>`

   * - **Use workshops**
     - :ref:`Connect VS Code <how_vscode_connect_remote>` •
       :ref:`JetBrains Gateway <how_jetbrains_gateway>` •
       :ref:`JupyterLab in browser <how_jupyterlab_run_in_browser>` •
       :ref:`Manage Python environments <how_manage_python_environments>` •
       :ref:`Use with Git <how_git_workshops>`

   * - **Create SDKs**
     - :ref:`Build an SDK <how_build_sdk>` •
       :ref:`Declare plugs and slots <how_declare_plugs_slots>` •
       :ref:`Configure a mount <how_configure_mount>` •
       :ref:`Write runtime hooks <how_write_runtime_hooks>` •
       :ref:`Design best practices <exp_sdk_best_practices>` •
       :ref:`Publish an SDK <how_publish_sdk>` •
       :ref:`Project definition file <ref_sdkcraft_definition>`

   * - **CI/CD**
     - :ref:`Run GitHub Actions locally <how_run_github_actions_locally>` •
       :ref:`Run workshops in GitHub Actions <how_run_workshops_in_github_actions>`

   * - **AI agents**
     - :ref:`Use with AI agents <how_use_workshops_with_ai_agents>` •
       :ref:`LLM-readable docs <ref_ai_discovery>` •
       :ref:`Workshop skill <ref_ai_use_workshop_skill>` •
       :ref:`SDK designer skill <ref_ai_sdk_designer_skill>`

   * - **Troubleshooting**
     - :ref:`Debug workshops <how_debug_issues_workshops>` •
       :ref:`Fix installation <how_troubleshoot>` •
       :ref:`Resolve plug conflicts <how_resolve_plug_conflicts>` •
       :ref:`Purge workshops <how_purge>`

   * - **Architecture**
     - :ref:`Components <exp_arch_system_components>` •
       :ref:`Runtime behavior <exp_arch_runtime_behavior>` •
       :ref:`Workshop internals <ref_workshop_internals>` •
       :ref:`SDK internals <ref_sdk_internals>`

   * - **CLI**
     - :ref:`Concepts <exp_cli>` •
       :ref:`Workshop CLI <ref_workshop__cli>` •
       :ref:`SDK CLI <ref_sdk__cli>` •
       :ref:`SDKcraft CLI <ref_sdkcraft__cli>` •
       :ref:`workshopctl CLI <ref_workshopctl__cli>`


How this documentation is organized
-----------------------------------

This documentation follows the `Diátaxis documentation framework <https://diataxis.fr/>`_,
organizing content by the type of information users need.
The four sections serve different purposes:

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
`Craft CLI <https://craft-cli.readthedocs.io/en/latest/>`_.

.. rubric:: Get involved

- :ref:`Contribute <contributing>`
- :ref:`Contribute to development <contributing_development>`
- :ref:`Contribute to this documentation <contributing_documentation>`

.. rubric:: Releases and roadmap

- :ref:`Release notes <release_notes>`

.. rubric:: Governance and policies

- `Code of conduct <https://ubuntu.com/community/docs/ethos/code-of-conduct>`__
- :doc:`Security policy </security>`
- `License <https://github.com/canonical/workshop/blob/main/LICENSE>`__

.. rubric:: Feedback and support

- `Product and documentation feedback <https://github.com/canonical/workshop/issues>`__
