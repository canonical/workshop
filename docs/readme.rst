Workshop
========

**Secure, fast, and composable development environments that come agent-ready.**

Workshop wraps complex, error-prone workspaces
into reproducible definitions of languages, libraries, and tooling.

- **Composable**: definitions are built from SDKs, independent units of
  functionality that publishers share on the SDK Store.
- **Transactional**: environment updates apply as changes you can inspect and restore.
- **Sandboxed**: experiment freely and tear down without leftover state.


Installation
------------

Workshop runs on Ubuntu and other ``snap``-enabled Linux distributions,
and requires `LXD 6.8+ <https://canonical.com/lxd>`_:

.. code-block:: console

   sudo snap install --channel=6/stable lxd  # skip if LXD is already installed
   sudo snap install --classic workshop


Quick start
-----------

In your project directory, define a workshop from a comma-separated list of SDKs:

.. code-block:: console

   workshop init dev --sdks opencode,go/1.26/stable


This writes ``.workshop/dev.yaml``, which you can extend with named actions:

.. code-block:: yaml

   # .workshop/dev.yaml
   name: dev
   base: ubuntu@24.04
   sdks:
     - name: opencode
     - name: go
       channel: 1.26/stable
   actions:                # add your own
     analyzer: |
       go vet ./...


Launch the workshop, then work inside it:

.. code-block:: console

   workshop launch       # download and install the SDKs
   workshop shell        # open an interactive session
   workshop run -- lint  # run a named action
   workshop refresh      # apply edits to the definition, update SDKs


SDKs
----

The Workshop team maintains a collection of
`reference SDKs <https://github.com/canonical/reference-sdks>`_
for languages, runtimes, AI agents, GPUs, and more,
including ``go``, ``node``, ``rust``, ``flutter``, and ``uv``.

Where an SDK follows an upstream release line, its channel tracks mirror it:

.. code-block:: console

   workshop init web --sdks node            # latest/stable, the default
   workshop init web --sdks node/24/stable  # pinned to the Node.js 24 LTS line
   workshop init api --sdks go/1.25/stable  # pinned to the Go 1.25 release line


Documentation
-------------

- `Tutorial <https://ubuntu.com/workshop/docs/tutorial/>`_: a detailed introduction to Workshop.
- `SDK crafting guide <https://ubuntu.com/workshop/docs/tutorial/part-4-craft-sdks/>`_:
  authoring SDKs with `SDKcraft <https://github.com/canonical/sdkcraft/>`_.


Community and support
---------------------

- `Code of conduct <https://ubuntu.com/community/docs/ethos/code-of-conduct>`_
- `Discourse <https://discourse.ubuntu.com/>`_
- `Product and documentation feedback <https://github.com/canonical/workshop/issues/>`_


Contributions and license
-------------------------

To join the development effort, see `How to contribute <contributing.rst>`_.

Workshop is released under the `GPL-3.0 license <../LICENSE>`_,
and the documentation under
`CC-BY-SA 4.0 <https://creativecommons.org/licenses/by-sa/4.0/>`_.
