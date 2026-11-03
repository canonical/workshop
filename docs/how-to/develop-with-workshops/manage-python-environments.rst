.. _how_manage_python_environments:

.. meta::
   :description: How-to guide on using the uv SDK to manage Python projects
                 and virtual environments inside a workshop, including pinning
                 the project venv with UV_PROJECT_ENVIRONMENT.

How to manage Python environments with the uv SDK
=================================================

.. @tests in tests/docs-how-to/manage-python-environments/task.yaml

.. @artefact uv SDK

The :samp:`uv` SDK is the recommended way to manage Python projects in |ws_markup|.
It ships :program:`uv` and :program:`uvx`,
aliases :program:`pip` to :program:`uv pip` for compatibility,
and exposes a virtual environment slot
that other Python-based SDKs (such as :samp:`jupyter`) can plug into.
The typical day-to-day flow boils down to running :program:`uv` commands
against a project virtual environment whose location you control.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- A Python project in the directory you launch the workshop from,
  with a :file:`pyproject.toml` or :file:`requirements.txt`
  that :program:`uv` can read.


Add the uv SDK to your workshop
-------------------------------

Add :samp:`- name: uv` to the workshop definition;
the SDK tracks the :samp:`latest/stable` channel by default,
which is sufficient for most projects:

.. code-block:: yaml
   :caption: workshop.yaml

   name: pyenv
   base: ubuntu@24.04
   sdks:
     - name: uv


Place your project at the host directory
that you launch the workshop from;
|ws_markup| bind-mounts it inside the workshop at :file:`/project/`.


Run uv from your project
------------------------

Open a shell in the workshop and use :program:`uv` from :file:`/project/`:

.. code-block:: console

   $ workshop shell
   workshop@pyenv:~$ cd /project
   workshop@pyenv:/project$ uv sync
   workshop@pyenv:/project$ uv add requests
   workshop@pyenv:/project$ uv run python -c "import requests"


:command:`uv sync` resolves the project's dependencies
and creates a virtual environment at :file:`/project/.venv`,
that is, next to your :file:`pyproject.toml`.
:command:`uv run` and :command:`uv add` then operate against that environment.
The venv lives on the host filesystem
because :file:`/project/` is the bind mount,
so it survives container refreshes
and is visible to host-side tooling such as IDEs.

The :samp:`uv` SDK aliases :program:`pip` to :program:`uv pip`
through :program:`update-alternatives`;
running :command:`pip install <PACKAGE>` from a workshop shell
transparently invokes :command:`uv pip install <PACKAGE>`.


Inspect what the SDK configures
-------------------------------

The SDK applies a few defaults
so that :program:`uv` works correctly with the workshop's storage layout:

.. @artefact UV_LINK_MODE

- :envvar:`UV_LINK_MODE` is set to :samp:`copy` in the workshop user's environment
  because the persistent cache mount does not support hard links.
  You don't need to set this yourself.

- The :program:`uv` package cache is persisted on the host
  through a :samp:`mount` interface plug
  that maps :file:`/home/workshop/.cache/uv/` to durable storage,
  so cached downloads survive workshop updates.

- A shared virtual environment slot is exposed
  at :file:`/home/workshop/uv-venv/`.
  This slot exists for cross-SDK sharing
  (see the next section);
  it is *not* the venv your project uses by default.


.. _how_manage_python_environments_share:

Share the environment with another SDK
--------------------------------------

To run a Python SDK such as JupyterLab against a uv-managed environment,
add the :samp:`jupyter` SDK alongside :samp:`uv`
and connect :samp:`jupyter:venv` to :samp:`uv:venv` explicitly
in the workshop definition:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 5,6-8

   name: pyenv
   base: ubuntu@24.04
   sdks:
     - name: uv
     - name: jupyter
   connections:
     - plug: jupyter:venv
       slot: uv:venv


An explicit :samp:`connections:` block is required:
without it, :samp:`jupyter:venv` falls back to :samp:`system:mount`
(the host directory |ws_markup| provides as a default plug target)
and the two SDKs don't share an environment.

After :command:`workshop refresh`,
:command:`workshop connections --all` confirms the wiring:

.. code-block:: console

   $ workshop connections --all

     INTERFACE  PLUG                SLOT                NOTES
     mount      pyenv/jupyter:venv  pyenv/uv:venv       -
     mount      pyenv/uv:cache      pyenv/system:mount  -


Packages that :samp:`jupyter` installs into its venv
now land in the shared environment provided by :samp:`uv`,
so :program:`jupyter` and :program:`uv run` see the same dependency set.


.. _how_manage_python_environments_pin:

Pin the project venv with UV_PROJECT_ENVIRONMENT
------------------------------------------------

By default, :program:`uv` places the project virtual environment
at :file:`.venv` next to the :file:`pyproject.toml` it discovers,
which inside a workshop is normally :file:`/project/.venv`.

Override this default with :envvar:`UV_PROJECT_ENVIRONMENT`
when you want a single, explicit venv location
regardless of where in the project tree :program:`uv` is invoked,
or when several workshops share the same project directory
and should reuse one venv.

For example, to place the venv at :file:`/project/pinned-venv`
instead of the default :file:`/project/.venv`:

.. code-block:: console

   $ echo 'export UV_PROJECT_ENVIRONMENT=/project/pinned-venv' >> ~/.profile
   $ exec bash -l
   $ cd /project
   $ uv sync


Relative paths are resolved from the workspace root,
absolute paths are used as is;
if the environment does not exist at the specified path,
:program:`uv` creates it.

.. warning::

   Set :envvar:`UV_PROJECT_ENVIRONMENT` to a path *inside* :file:`/project/`,
   such as :file:`/project/.venv/`,
   not to :file:`/project/` itself.
   :program:`uv` writes the venv layout
   (:file:`bin/`, :file:`lib/`, :file:`pyvenv.cfg`)
   directly under the value you provide,
   so a bare :file:`/project/`
   would scatter venv files across your project sources.


See also
--------

Explanation:

- :ref:`exp_best_dependencies`
- :ref:`exp_mount_interface`
- :ref:`exp_workshop_definition`


How-to guides:

- :ref:`how_jupyterlab_run_in_browser`
