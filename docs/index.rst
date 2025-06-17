:slug: home-page
:relatedlinks: [Workshop](https://github.com/canonical/workshop/), [SDKcraft](https://github.com/canonical/sdkcraft/), [LXD](https://documentation.ubuntu.com/lxd/), [Snap](https://snapcraft.io/docs/)

.. _home:

|ws_markup|
===========

.. toctree::
   :hidden:

   Home <self>
   tutorial/index
   how-to/index
   reference/index
   explanation/index
   Contribution <contributing>
   Release notes <release-notes>


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

.. grid:: 1 1 2 2

   .. grid-item:: :doc:`Tutorial <tutorial/index>`

      :ref:`Install <tut_install>` |
      :ref:`Define workshops <tut_define_launch>` |
      :ref:`Refresh workshops <tut_refresh>` |
      :ref:`Run commands <tut_exec>` |
      :ref:`Use interfaces <tut_interfaces>`


   .. grid-item:: :doc:`How-to guides <how-to/index>`

      **Step-by-step guides** for common |ws_markup| and |sdk_markup| tasks


   .. grid-item:: :doc:`Explanation <explanation/index>`

      **Discussion and clarification** of key topics


   .. grid-item:: :doc:`Reference <reference/index>`

      **Technical details**, specifications, APIs


- **Projects**: :ref:`Concepts <exp_projects>` |
  :ref:`Move projects <how_move_projects>` |
  :ref:`Update projects <tut_project_updates>`

- **Workshops**: :ref:`Concepts <exp_workshop_concepts>` |
  :ref:`Internals <ref_workshop_internals>` |
  :ref:`Definition file <ref_workshop_definition>` |
  :ref:`Status diagrams <ref_workshop_status>` |
  :ref:`Debug workshops <how_debug_issues_workshops>`

- **SDKs**: :ref:`Concepts <exp_sdk_concepts>` |
  :ref:`Internals <ref_sdk_internals>` |
  :ref:`Definition file <ref_sdk_definition>` |
  :ref:`Design SDKs <how_create_ros2_sdk>` |
  :ref:`Sketch SDKs in-place <how_sketch>` |
  :ref:`Craft full SDKs <how_sdkcraft>`

- **Interfaces**: :ref:`Concepts <exp_interface_concepts>` |
  :ref:`Camera <exp_camera_interface>` |
  :ref:`Desktop <exp_desktop_interface>` |
  :ref:`GPU <exp_gpu_interface>` |
  :ref:`Mounts <exp_mount_interface>` |
  :ref:`SSH agent <exp_ssh_interface>` |
  :ref:`Networking <exp_tunnel_interface>`

- **Utilities**: :ref:`Command-line tools <ref_cli>` |
  :ref:`Troubleshoot installation <how_troubleshoot>`

----

.. _project_community:

Project and community
---------------------

|ws_markup| is an emergent project
within the Enterprise Engineering department here at Canonical;
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


- `Code of conduct <https://ubuntu.com/community/ethos/code-of-conduct>`__

- `Pulse reviews on Discourse <https://discourse.canonical.com/c/engineering/sdk/34>`__

- `Mattermost channel <https://chat.canonical.com/canonical/channels/sdk>`__

- `Product map <https://warthogs.atlassian.net/jira/software/c/projects/WSP/boards/1645>`__

- :ref:`Contribution and participation <contributing>`

- :ref:`Release notes <release_notes>`

- `Product and documentation feedback <https://github.com/canonical/workshop/issues>`__
