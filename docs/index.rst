:slug: home-page
:relatedlinks: [Workshop repo](https://github.com/canonical/workshop/), [SDKcraft repo](https://github.com/canonical/sdkcraft/), [Example workshop repositories](https://github.com/canonical/reference-workshops), [Example SDK repositories](https://github.com/canonical/reference-sdks), [LXD docs](https://canonical.com/lxd/docs/default/), [Snap docs](https://snapcraft.io/docs/)

.. _home:

.. meta::
   :description: Find Workshop documentation by subject: getting started, concepts and
                 architecture, using workshops, workshop configuration, SDK development,
                 shared resources, tool integrations, maintenance, security, and use cases.

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



|ws_markup| is a tool for creating and managing container-based development
environments. Define the languages, libraries, and tools your project needs
in a workshop definition, then compose the environment from SDKs:
independent units of functionality that connect through interfaces.
Use SDKs published on the SDK Store or define them in your repository.

Workshop helps developers and teams reproduce complex setups across machines,
experiment in a sandbox, and manage environment updates.
It supports projects that span Ubuntu versions, container images, languages,
and frameworks, including AI and machine learning, robotics, IoT,
and education. Reuse a definition to set up an environment
or rebuild it when you need a fresh start.

----



.. _documentation_directory:

In this documentation
---------------------

.. _home_getting_started:

.. _start-here:

Getting started
~~~~~~~~~~~~~~~

.. domain::

   .. slice:: Installation

      :ref:`Install Workshop <tut_install>`

   .. slice:: Tutorial

      :ref:`Get started with Workshop <tut_get_started>`
      :ref:`Work with interfaces <tut_interfaces>`
      :ref:`Sketch SDKs <tut_sketch_sdks>`
      :ref:`Craft SDKs <tut_craft_sdks>`

.. _home_key_concepts_and_architecture:

.. _know-your-workshop:

.. _key-concepts-and-architecture:

.. _key-concepts-and-architecture-heading:

Concepts and architecture
~~~~~~~~~~~~~~~~~~~~~~~~~

.. domain::

   .. slice:: Workshops

      :ref:`Workshop concepts <exp_workshop_concepts>`
      :ref:`Changes and tasks explained <exp_changes_tasks>`
      :ref:`How workshop actions work <exp_workshop_definition_actions>`
      :ref:`Workshop status meanings <exp_workshop_status>`
      :ref:`Workshop states and transitions <exp_workshop_lifecycle>`
      :ref:`Workshop base <exp_base>`

   .. slice:: Projects

      :ref:`Project structure <exp_projects>`
      :ref:`Multi-workshop patterns <exp_multi_workshop_patterns>` domain

   .. slice:: SDKs

      :ref:`SDK concepts <exp_sdk_concepts>`
      :ref:`Build and runtime definitions <exp_sdk_definition>`
      :ref:`SDK parts <exp_sdk_parts>`
      :ref:`Runtime hook model <exp_sdk_hooks>` domain
      :ref:`SDK development and distribution <exp_sdk_lifecycle>`

   .. slice:: SDK sources

      :ref:`System SDK <exp_system_sdk>`
      :ref:`SDK Store <exp_sdk_store>`
      :ref:`In-project SDKs <exp_in_project_sdk>`
      :ref:`SDK sketches <exp_sketch_sdk>`
      :ref:`SDK channels <ref_sdk_channels>`

   .. slice:: Interfaces

      :ref:`Interface concepts <exp_interface_concepts>`
      :ref:`Plugs and slots <exp_plugs_slots>`
      :ref:`Auto-connection <exp_interface_auto_connection>`
      :ref:`Connections <exp_interface_connections>`
      :ref:`Plug bindings <exp_plug_bindings>`
      :ref:`Validation <exp_interfaces_validation>`
      :ref:`Connection persistence <exp_workshop_connection_lifecycle>`

   .. slice:: CLI concepts

      :ref:`Command-line tool roles <exp_cli>`

   .. slice:: Architecture

      :ref:`System components <exp_arch_system_components>`
      :ref:`Runtime behavior <exp_arch_runtime_behavior>`
      :ref:`Daemon <exp_arch_daemon>`
      :ref:`REST API <exp_arch_api>`
      :ref:`LXD backend <exp_arch_lxd_backend>`
      :ref:`Storage backends <exp_arch_zfs_storage>`
      :ref:`State database <exp_arch_state_database>`
      :ref:`Images <exp_arch_images>`
      :ref:`Network <exp_arch_network>`
      :ref:`Launch process <exp_arch_workshop_launch>`
      :ref:`Container layout <exp_arch_container_layout>`
      :ref:`Workshop internals <ref_workshop_internals>`
      :ref:`SDK internals <ref_sdk_internals>`

.. _home_using_workshops:

Using workshops
~~~~~~~~~~~~~~~

.. domain::

   .. slice:: Inspection

      :ref:`Workshop transition diagrams <ref_workshop_status>`
      :ref:`workshop info <ref_workshop_info>`
      :ref:`workshop list <ref_workshop_list>`

   .. slice:: Container controls

      :ref:`workshop launch <ref_workshop_launch>`
      :ref:`workshop remove <ref_workshop_remove>`
      :ref:`workshop start <ref_workshop_start>`
      :ref:`workshop stop <ref_workshop_stop>`

   .. slice:: Refresh and restore

      :ref:`workshop refresh <ref_workshop_refresh>`
      :ref:`workshop restore <ref_workshop_restore>`

   .. slice:: Commands and shells

      :ref:`workshop exec <ref_workshop_exec>`
      :ref:`workshop shell <ref_workshop_shell>`

   .. slice:: Actions

      :ref:`Add actions <how_add_actions>`
      :ref:`workshop actions <ref_workshop_actions>`
      :ref:`workshop run <ref_workshop_run>`

   .. slice:: CLI tools

      :ref:`workshop CLI reference <ref_workshop__cli>`
      :ref:`workshopctl CLI reference <ref_workshopctl__cli>`
      :ref:`Shell completion <ref_workshop__cli_completion>`

.. _home_workshop_configuration:

.. _work-in-a-workshop:

Workshop configuration
~~~~~~~~~~~~~~~~~~~~~~

.. domain::

   .. slice:: Definitions

      :ref:`workshop.yaml configuration reference <ref_workshop_definition>`
      :ref:`workshop init <ref_workshop_init>`
      `Example workshop repositories <https://github.com/canonical/reference-workshops>`__

   .. slice:: SDK selection

      :ref:`How SDKs compose a workshop <exp_workshop_definition_sdks>`
      :ref:`sdk CLI reference <ref_sdk__cli>`
      :ref:`sdk find <ref_sdk_find>`
      :ref:`sdk info <ref_sdk_info>`
      :ref:`sdk list <ref_sdk_list>`

   .. slice:: SDK sketches

      :ref:`workshop sketch-sdk <ref_workshop_sketch-sdk>`
      :ref:`workshop sketches <ref_workshop_sketches>`

   .. slice:: Interface connections

      :ref:`Plugs, slots, and connections <exp_workshop_definition_connections>`
      :ref:`Connect interfaces with the CLI <exp_interfaces_cli_operations>`
      :ref:`workshop connect <ref_workshop_connect>`
      :ref:`workshop connections <ref_workshop_connections>`
      :ref:`workshop disconnect <ref_workshop_disconnect>`
      :ref:`workshop remount <ref_workshop_remount>`

   .. slice:: Project organisation

      :ref:`Project updates <tut_project_updates>`
      :ref:`Use multiple workshops <how_use_multiple_workshops>` domain
      :ref:`Move projects <how_move_projects>`

.. _home_sdk_development:

.. _craft-and-publish-sdks:

SDK development
~~~~~~~~~~~~~~~

.. domain::

   .. slice:: SDK design

      :ref:`SDK best practices <exp_sdk_best_practices>`
      :ref:`SDKs vs Dockerfiles <exp_dockerfile_vs_sdk>`
      `Example SDK repositories <https://github.com/canonical/reference-sdks>`__

   .. slice:: Definition files

      :ref:`sdkcraft.yaml build definition <ref_sdkcraft_definition>`
      :ref:`sdk.yaml runtime definition <ref_sdk_definition>`
      :ref:`sdkcraft init <ref_sdkcraft_init>`

   .. slice:: Packaging

      :ref:`Build an SDK <how_build_sdk>`
      :ref:`sdkcraft CLI reference <ref_sdkcraft__cli>`
      :ref:`sdkcraft pack <ref_sdkcraft_pack>`

   .. slice:: Build controls

      :ref:`sdkcraft build <ref_sdkcraft_build>`
      :ref:`sdkcraft clean <ref_sdkcraft_clean>`
      :ref:`sdkcraft prime <ref_sdkcraft_prime>`
      :ref:`sdkcraft pull <ref_sdkcraft_pull>`
      :ref:`sdkcraft stage <ref_sdkcraft_stage>`

   .. slice:: Runtime hooks

      :ref:`Runtime hook model <exp_sdk_hooks>` domain
      :ref:`Write runtime hooks <how_write_runtime_hooks>`

   .. slice:: Resource interfaces

      :ref:`Content sharing <exp_content_sharing>`
      :ref:`Declare plugs and slots <how_declare_plugs_slots>`
      :ref:`Configure a mount <how_configure_mount>`
      :ref:`Share content between SDKs <how_share_content_between_sdks>`

   .. slice:: Testing and health

      :ref:`SDK testing options <exp_test_try_sdk>`
      :ref:`Health reporting <exp_workshopctl_health>`
      :ref:`Try an SDK in a workshop <how_build_sdk_try>`
      :ref:`sdkcraft test <ref_sdkcraft_test>`
      :ref:`sdkcraft try <ref_sdkcraft_try>`

   .. slice:: Publishing

      :ref:`Publish an SDK <how_publish_sdk>`
      :ref:`Automate SDK uploads from CI <how_publish_sdk_ci>` domain
      :ref:`sdkcraft release <ref_sdkcraft_release>`
      :ref:`sdkcraft upload <ref_sdkcraft_upload>`

   .. slice:: Store administration

      :ref:`sdkcraft create-track <ref_sdkcraft_create_track>`
      :ref:`sdkcraft login <ref_sdkcraft_login>`
      :ref:`sdkcraft register <ref_sdkcraft_register>`
      :ref:`sdkcraft revisions <ref_sdkcraft_revisions>`

.. _home_shared_resources:

.. _connect-tools-and-resources:

Shared resources
~~~~~~~~~~~~~~~~

.. domain::

   .. slice:: Files and storage

      :ref:`Mount interface <exp_mount_interface>`
      :ref:`Mount interface reference <ref_mount_interface>`
      :ref:`Add mounts <how_add_mounts>`
      :ref:`Reset a remount <how_reset_remount>`
      :ref:`Storage pools <ref_workshop_storage_pools>`

   .. slice:: Hardware

      :ref:`GPU interface <exp_gpu_interface>` domain
      :ref:`GPU interface reference <ref_gpu_interface>`
      :ref:`Camera interface <exp_camera_interface>` domain
      :ref:`Camera interface reference <ref_camera_interface>`
      :ref:`Custom device interface <exp_custom_device_interface>`
      :ref:`Custom device interface reference <ref_custom_device_interface>`
      :ref:`Use host devices <how_use_host_devices>` domain

   .. slice:: Display

      :ref:`Desktop interface <exp_desktop_interface>`
      :ref:`Desktop interface reference <ref_desktop_interface>`

   .. slice:: Networking

      :ref:`Tunnel interface <exp_tunnel_interface>`
      :ref:`Tunnel interface reference <ref_tunnel_interface>`
      :ref:`Tunnel auto-connection rules <exp_tunnel_connection>`
      :ref:`Workshop hostnames <exp_workshop_hostname>`
      :ref:`Forward ports <how_forward_ports>`
      :ref:`Cross-workshop networking <how_use_multiple_workshops_networking>` domain

   .. slice:: SSH agent

      :ref:`SSH agent interface <exp_ssh_interface>`
      :ref:`SSH agent interface reference <ref_ssh_interface>`

.. _home_tool_integrations:

Tool integrations
~~~~~~~~~~~~~~~~~

.. domain::

   .. slice:: Editors and IDEs

      :ref:`Develop in VS Code <how_vscode_connect_remote>`
      :ref:`Connect JetBrains Gateway <how_jetbrains_gateway>`
      :ref:`Run JupyterLab in a browser <how_jupyterlab_run_in_browser>`

   .. slice:: Python environments

      :ref:`Manage Python environments <how_manage_python_environments>` domain

   .. slice:: Version control

      :ref:`Use with Git <how_git_workshops>`

   .. slice:: Continuous integration

      :ref:`Run GitHub Actions locally <how_run_github_actions_locally>` domain
      :ref:`Run workshops in GitHub Actions <how_run_workshops_in_github_actions>` domain

   .. slice:: AI agents

      :ref:`Use with AI agents <how_use_workshops_with_ai_agents>` domain
      :ref:`AI agent reference <ref_ai_agents>`
      :ref:`use-workshop skill <ref_ai_use_workshop_skill>`
      :ref:`onboard-workshop skill <ref_ai_onboard_workshop_skill>`
      :ref:`design-sdk skill <ref_ai_design_sdk_skill>`
      :ref:`LLM-readable docs <ref_ai_discovery>`
      :ref:`Context7 <ref_ai_context7>`

.. _home_maintenance:

.. _maintain-and-secure:

Maintenance
~~~~~~~~~~~

.. domain::

   .. slice:: Releases and compatibility

      :ref:`Release policy and LTS <release_policy>`
      :ref:`Backward compatibility <exp_workshop_backward_compat>`
      :ref:`Forward compatibility <ref_workshop_forward_compat>`
      :ref:`Upgrade Workshop <release_upgrade>`

   .. slice:: Diagnostics

      :ref:`Debug issues <how_debug_issues_workshops>`
      :ref:`Track workshop changes <tut_changes_tasks>`
      :ref:`workshop changes <ref_workshop_changes>`
      :ref:`workshop okay <ref_workshop_okay>`
      :ref:`workshop tasks <ref_workshop_tasks>`
      :ref:`workshop warnings <ref_workshop_warnings>`

   .. slice:: Recovery

      :ref:`Resolve plug conflicts <how_resolve_plug_conflicts>`
      :ref:`Fix the installation <how_troubleshoot>`
      :ref:`Purge workshops <how_purge>`

.. _home_security:

Security
~~~~~~~~

.. domain::

   .. slice:: Permissions and isolation

      :ref:`Privileges <security_privileges>`
      :ref:`Isolation <security_isolation>`

   .. slice:: Data and SDK trust

      :ref:`Sensitive data and SDK trust <security_risks>`

   .. slice:: CI runners

      :ref:`Local runner security <how_run_github_actions_locally_security>`
      :ref:`GitHub Action security <how_run_workshops_in_github_actions_security>`

.. _home_use_cases:

.. _home_development_scenarios:

.. _development-scenarios:

Use cases
~~~~~~~~~

.. domain::

   .. slice:: AI agents

      :ref:`Use with AI agents <how_use_workshops_with_ai_agents>` domain
      :ref:`Parallel agent runs <how_ai_agents_parallel_runs>`
      :ref:`Role-based coding <how_ai_agents_role_based>`

   .. slice:: AI/ML and data science

      :ref:`GPU interface <exp_gpu_interface>` domain
      :ref:`Jupyter with a uv environment <tut_jupyter_uv_venv>`
      :ref:`Manage Python environments <how_manage_python_environments>` domain

   .. slice:: Robotics and embedded

      :ref:`ROS 2 case study <exp_ros2_case_study>`
      :ref:`Camera interface <exp_camera_interface>` domain
      :ref:`Use host devices <how_use_host_devices>` domain

   .. slice:: CI/CD

      :ref:`Run GitHub Actions locally <how_run_github_actions_locally>` domain
      :ref:`Run workshops in GitHub Actions <how_run_workshops_in_github_actions>` domain
      :ref:`Automate SDK uploads from CI <how_publish_sdk_ci>` domain

   .. slice:: Multiple workshops

      :ref:`Multi-workshop patterns <exp_multi_workshop_patterns>` domain
      :ref:`Use multiple workshops <how_use_multiple_workshops>` domain
      :ref:`Cross-workshop networking <how_use_multiple_workshops_networking>` domain

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
`Craft CLI <https://craft-cli.readthedocs.io/en/latest/>`_.

.. rubric:: Get involved

- :ref:`Contribution overview <contributing>`
- :ref:`Contribute to development <contributing_development>`
- :ref:`Contribute to documentation <contributing_documentation>`

.. rubric:: Releases

- :ref:`Release notes <release_notes>`

.. rubric:: Governance and policies

- `Code of conduct <https://ubuntu.com/community/docs/ethos/code-of-conduct>`__
- :doc:`Security policy </security>`
- `License <https://github.com/canonical/workshop/blob/main/LICENSE>`__

.. rubric:: Feedback and support

- `Product and documentation feedback <https://github.com/canonical/workshop/issues>`__
- :ref:`Report a vulnerability <security_reporting>`
