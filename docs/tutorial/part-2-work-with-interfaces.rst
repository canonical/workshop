.. _tut_work_with_interfaces:

.. meta::
   :description: Tutorial on using interfaces in workshops
                 to interact with the host system and other SDKs.

.. _tut_interfaces:

Work with interfaces
====================

.. @tests in tests/docs-tutorial/part-2/task.yaml

This is the second section of the :ref:`four-part series <tut_index>`;
it explains how to work with interfaces.
It relies on the knowledge gained in the :ref:`tut_get_started` section,
where you learned how to create and run workshops.
Here, you will learn how to make better use of SDKs in your workshop
and integrate them with the host system.

.. @artefact interface
.. @artefact system SDK

SDKs use interfaces to interact in an organized manner,
exposing the resources they provide via slots and consuming them via plugs;
the layout of these plugs and slots is defined by the SDK publishers.

For uniformity, security, and control,
various host system capabilities (camera, GPU, and so on)
are also exposed to the workshop via the same interface mechanism
with a designated system SDK.


Manage connections
------------------

To check out the connected interfaces of a workshop, list the connections:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections

     INTERFACE  PLUG               SLOT              NOTES
     gpu        dev/ollama:gpu     dev/system:gpu    -
     mount      dev/ollama:models  dev/system:mount  -


This lists two interface plugs,
both provided by the :samp:`ollama` SDK under the :samp:`dev` workshop.

The first one is a GPU interface plug named :samp:`dev/ollama:gpu`.
It enables the workshop to use the host system's GPU
by connecting to the :samp:`dev/system:gpu` slot.

Also, there's a mount interface plug named :samp:`dev/ollama:models`.
As seen in the :command:`workshop info` output,
it was automatically connected at :ref:`launch <tut_launch>`
to the :samp:`dev/system:mount` slot,
indicated by the ellipsis in the :samp:`host-source` path.

Note that some interfaces are auto-connected, while some are not;
this depends on their built-in security policy defined by |ws_markup|.
For instance, you can't use the ssh-agent interface
without connecting it manually.

In any case, you can connect and disconnect interfaces at will.
To check the connection state, run :command:`workshop connections`:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop disconnect dev/ollama:models
   $ workshop connections

     INTERFACE  PLUG            SLOT            NOTES
     gpu        dev/ollama:gpu  dev/system:gpu  -

   $ workshop connect dev/ollama:models :mount
   $ workshop connections

     INTERFACE  PLUG               SLOT              NOTES
     gpu        dev/ollama:gpu     dev/system:gpu    -
     mount      dev/ollama:models  dev/system:mount  manual


You can remount a mount interface plug to a new location on the host.
For example, to preserve models,
conventionally stored under :file:`~/.ollama/models/`,
after the workshop is removed or use some models downloaded previously,
you can remount to a directory in your home:

.. @artefact workshop remount

.. code-block:: console
   :emphasize-lines: 16-19

   $ mkdir -p ~/.ollama/models
   $ workshop remount dev/ollama:models ~/.ollama/models
   $ workshop info

     name:     dev
     base:     ubuntu@24.04
     project:  /home/user/ollama-python-project
     status:   ready
     notes:    -
     sdks:
       system:
         installed:  (1)
       ollama:
         tracking:   vulkan/stable
         installed:  0.9.6  2025-11-19  (214)
         mounts:
           models:
             host-source:      /home/user/.ollama/models
             workshop-target:  /home/workshop/.ollama/models


This makes :file:`/home/user/.ollama/models/` on the host
act as the models storage for the workshop.


Add plugs, slots
----------------

You can modify the behavior of the SDKs you installed in your workshop,
tailoring it to your needs and connecting them to other SDKs or the host system.

To do this, you add plugs and slots to the SDKs in the workshop definition,
allowing you to customize the initial plug and slot layout to your requirements.

This scenario usually arises
when you want to connect different SDKs running in the workshop
or expose some service from the workshop to the host system.

Let's look at an example.
Add the :samp:`jupyter` SDK to the workshop
to run Jupyter notebooks with the Ollama models:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 6

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: vulkan/stable
     - name: jupyter


.. @artefact tunnel interface

Next, add the tunnel interface plug under the :samp:`system` SDK
in the workshop definition;
this exposes the Jupyter server, now available in the workshop,
to the host system at a port of your choice (here, :samp:`8989`):

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 7-11

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: vulkan/stable
     - name: jupyter
     - name: system
       plugs:
         jupyter:
           interface: tunnel
           endpoint: 127.0.0.1:8989


The slot we're going to connect this plug to comes from the SDK itself
and is named :samp:`jupyter`,
so you don't have to add it manually.

Refresh the workshop to enable the tunnel;
|ws_markup| will auto-connect the plug to the slot by matching their names
(the plug's name is also :samp:`jupyter`).
Check the result using :command:`workshop connections` or :command:`workshop info`:

.. code-block:: console
   :emphasize-lines: 24

   $ workshop refresh

     "dev" refreshed

   $ workshop info

     ...
     sdks:
       system:
         installed:  (1)
         tunnels:
           jupyter:
             from:  127.0.0.1:8989/tcp
             to:    127.0.0.1:8888/tcp
     ...

   $ workshop connections --all

     INTERFACE  PLUG                SLOT                      NOTES
     gpu        dev/ollama:gpu      dev/system:gpu            -
     mount      dev/jupyter:venv    dev/system:mount          -
     mount      dev/ollama:models   dev/system:mount          manual
     tunnel     -                   dev/ollama:ollama-server  -
     tunnel     dev/system:jupyter  dev/jupyter:jupyter       -


Now, JupyterLab is available at the plug address:

.. code-block:: console

   $ curl -w '\n' http://127.0.0.1:8989/api

     {"version": "2.17.0"}


.. note::

   For additional details of using the tunnel interface, see this guide:
   :ref:`how_forward_ports`.


.. _tut_jupyter_uv_venv:

Wire jupyter to a uv-managed Python environment
-----------------------------------------------

So far, :samp:`jupyter:venv` auto-connects to the :samp:`system:mount` slot,
which gives Jupyter a private host directory for its virtual environment.
A more interesting wiring uses the :samp:`uv` SDK,
the standard Python tooling SDK in |ws_markup|;
:samp:`uv` exposes a :samp:`venv` slot
that other Python-based SDKs can plug into,
so Jupyter and uv share a single environment.

Edit the workshop definition to add :samp:`uv`
*before* :samp:`jupyter` in the :samp:`sdks:` list,
so that :samp:`uv`'s :samp:`setup-project` hook
prepares the shared virtual environment
before any consuming SDK installs into it.
Then declare the connection in a top-level :samp:`connections:` block:

.. code-block:: yaml
   :caption: .workshop/dev.yaml
   :emphasize-lines: 6,13-15

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: vulkan/stable
     - name: uv
     - name: jupyter
     - name: system
       plugs:
         jupyter:
           interface: tunnel
           endpoint: 127.0.0.1:8989
   connections:
     - plug: jupyter:venv
       slot: uv:venv


Apply the new definition by refreshing the workshop:

.. code-block:: console

   $ workshop refresh


:samp:`dev/jupyter:venv` now connects to :samp:`dev/uv:venv`
instead of falling back to :samp:`dev/system:mount`:

.. code-block:: console
   :emphasize-lines: 6

   $ workshop connections --all

     INTERFACE  PLUG                SLOT                      NOTES
     gpu        dev/ollama:gpu      dev/system:gpu            -
     mount      dev/jupyter:venv    dev/uv:venv               -
     mount      dev/ollama:models   dev/system:mount          manual
     mount      dev/uv:cache        dev/system:mount          -
     tunnel     -                   dev/ollama:ollama-server  -
     tunnel     dev/system:jupyter  dev/jupyter:jupyter       -


This is your first taste of slot/plug coordination
between two nonsystem SDKs;
for the full Python workflow with :program:`uv`,
see :ref:`how_manage_python_environments`.


Next steps
----------

This was the last step in this tutorial section; you're halfway through!
Now you are familiar with the essentials of interfaces in |ws_markup|.

Your next step is to learn even more about workshop customization,
creating experimental SDKs quickly
with the :command:`workshop sketch-sdk` command;
proceed to the :ref:`tut_sketch_sdks` section.
