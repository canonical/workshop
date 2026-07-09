Workshop
========

**Workshops are secure, fast, and composable development environments that come agent-ready**.

**Wrap complex, error-prone workspaces
into reliable and reproducible definitions of languages, libraries, and tooling**.
The key pieces of a definition are SDKs:
independent, connectable units of functionality
that publishers package and share on the SDK Store,
and teams can define in their repositories.

**Workshops enable sandboxed experimentation,
turn environment updates into manageable transactions,
and ensure consistent, reproducible environments**.
With Workshop, you can launch a setup
that previously took hours to configure in a few commands
and be sure it will work the same way every time,
or tear it down and start from the last step without worrying about leftover state.

**Agentic engineering, AI/ML, robotics, IoT, EdTech, and similar domains**
typically use less-than-trivial project layouts
that rely on many Ubuntu versions or container images,
a plethora of diverse tools and frameworks,
and a wide range of libraries and languages.
That's where Workshop thrives.

**Built for AI workflows**.
Workshop publishes
`LLM-readable docs <https://ubuntu.com/workshop/docs/reference/ai-agents/#llm-readable-docs>`_,
and ships agentic skills for
`operating workshops <https://ubuntu.com/workshop/docs/reference/ai-agents/#the-use-workshop-skill>`_
and
`scaffolding SDKs <https://ubuntu.com/workshop/docs/reference/ai-agents/#the-sdk-designer-skill>`_.


Using Workshop
--------------

In the directory of the project
that you want to use with Workshop,
run ``workshop init`` with a comma-separated list of SDKs,
pinning any of them to a channel:

.. code-block:: console

   workshop init dev --sdks opencode,go/1.26/stable


This writes ``.workshop/dev.yaml``
with the ``opencode`` SDK on its default channel
and the ``go`` SDK pinned to ``1.26/stable``:

.. code-block:: yaml
   :caption: .workshop/dev.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: opencode
     - name: go
       channel: 1.26/stable


Launch the workshop:

.. code-block:: console

   workshop launch


Workshop downloads and installs the SDKs your definition lists;
the project is now ready to use them.


Reference SDKs
--------------

The Workshop team maintains a curated collection of
`reference SDKs <https://github.com/canonical/reference-sdks>`__:
ready-to-use components for languages, runtimes, AI agents, GPUs, and more.
In particular, language and runtime SDKs include
``go``, ``node``, ``rust``, ``flutter``, and ``uv``.

Add any of them to a workshop the same way.
For example, to start a Node.js LTS environment:

.. code-block:: console

   workshop init web --sdks node/24/stable
   workshop launch


Each SDK is published on the SDK Store through channels
of the form ``<TRACK>/<RISK>``, defaulting to ``latest/stable``.
Where an SDK follows an upstream release line, its tracks mirror it:
the ``node`` SDK exposes a track per Node.js LTS line
(``node/20/stable``, ``node/22/stable``, ``node/24/stable``),
and the ``go`` SDK a track per Go release line
(``go/1.24/stable``, ``go/1.25/stable``, ``go/1.26/stable``).
Pin a track to stay on that line,
or omit the channel to follow the latest stable release.


Installation
------------

Workshop is supported on Ubuntu and other ``snap``-enabled Linux distributions.

Prerequisites
~~~~~~~~~~~~~

Workshop requires
`LXD 6.8+ <https://canonical.com/lxd>`_
for low-level operation.

If the ``snap install`` command reports an issue with LXD,
install a recent LXD version with ``snap``:

.. code-block:: console

   sudo snap install --channel=6/stable lxd  # to install
   sudo snap refresh --channel=6/stable lxd  # to update


Install Workshop
~~~~~~~~~~~~~~~~

Install the snap using the
`--classic <https://snapcraft.io/docs/install-modes/>`_ option:

.. code-block:: console

   sudo snap install --classic workshop


The downside of this method is that you will need to manually
check for and install updates.

Documentation
-------------

Refer to the
`Tutorial
<https://ubuntu.com/workshop/docs/tutorial/>`_
in our docs for a detailed introduction to Workshop.

To know more about `SDKcraft <https://github.com/canonical/sdkcraft/>`_,
the SDK authoring tool for Workshop,
jump straight to the
`SDK crafting guide
<https://ubuntu.com/workshop/docs/tutorial/part-4-craft-sdks/>`_
in our docs.

For reference examples of SDK implementation, see the
`reference SDKs repository <https://github.com/canonical/reference-sdks>`__.


Community and Support
---------------------

Use the following resources for communication, support, and feedback:

- `Code of conduct <https://ubuntu.com/community/docs/ethos/code-of-conduct>`__

- `Discourse <https://discourse.ubuntu.com/>`__

- `Product and documentation feedback <https://github.com/canonical/workshop/issues/>`__


Contributions
-------------

To join the development effort, see `How to contribute <contributing.rst>`_.


License
-------

Workshop is released under the `GPL-3.0 license <../LICENSE>`_.

The documentation is licensed under
`CC-BY-SA 4.0 <https://creativecommons.org/licenses/by-sa/4.0/>`_.
