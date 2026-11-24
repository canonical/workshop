.. _how_jupyterlab_run_in_browser:

.. meta::
   :description: How-to guide on running a JupyterLab instance in a workshop
                 and accessing it via a browser.

How to run JupyterLab in your browser
=====================================

.. @tests made redundant by tests/docs-tutorial/part-3/task.yaml

A JupyterLab instance can run inside a workshop
and be accessed from your browser via the :samp:`jupyter` SDK.

To do that, add the :samp:`jupyter` SDK
and configure a tunnel interface plug for the :samp:`system` SDK:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 4-9

    name: dev
    base: ubuntu@24.04
    sdks:
    - name: system
      plugs:
        jupyter:
          interface: tunnel
          endpoint: 127.0.0.1:8989
    - name: jupyter


Launch the workshop.
After that, JupyterLab will be available in your browser at the plug address,
e.g., http://localhost:8989.
It starts as a user service
with :file:`/project/` as the default working directory to serve from.
You can immediately start using it with any other SDKs you have installed.


See also
--------

Explanation:

- :ref:`exp_system_sdk`
- :ref:`exp_tunnel_plug`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_tunnel_interface`
- :ref:`ref_workshop_launch`
