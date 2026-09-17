:hide-toc:
.. _how_resolve_plug_conflicts:

.. meta::
   :description: Step-by-step guide on binding interface plugs in a workshop to resolve resource conflicts between SDKs.

How to fix plug conflicts with binding
======================================

.. @tests made redundant by tests/main/interface-mount/task.yaml, tests/main/remount/task.yaml

.. @artefact mount interface
.. @artefact interface plug

The example below binds plugs for the :samp:`mount` interface,
but the same process works for any other interface that supports bindings.

Suppose you have two SDKs that each declare a plug of the same interface,
and the plugs are in conflict.
Here, we use fictional :samp:`torchaudio:hub` and :samp:`torchvision:hub`
that both point to the :file:`~/.cache/torch/hub` directory on the host,
where the SDKs store their models.

#. Create or open your workshop definition and list both SDKs:

   .. code-block:: yaml
      :caption: .workshop/digits.yaml

      name: digits
      base: ubuntu@22.04
      sdks:
        - name: torchaudio
        - name: torchvision


   Launching this workshop would cause a conflict
   because both SDKs want to mount the same directory in the workshop,
   which is not allowed.

.. @artefact plug binding

#. To address this issue,
   bind the :samp:`torchvision:hub` plug to the :samp:`torchaudio:hub` plug
   by adding a :samp:`bind` attribute in the workshop definition:

   .. code-block:: yaml
      :caption: .workshop/digits.yaml
      :emphasize-lines: 8

      name: digits
      base: ubuntu@22.04
      sdks:
        - name: torchaudio
        - name: torchvision
          plugs:
            hub:
              bind: torchaudio:hub


#. Launch the workshop.
   |ws_markup| now recognizes
   that :samp:`torchvision:hub` is bound to :samp:`torchaudio:hub`
   and therefore mounts a single directory for both plugs.

   .. code-block:: console

      $ workshop launch digits


#. Verify the binding with :command:`workshop connections`:

   .. code-block:: console

      $ workshop connections digits

        INTERFACE  PLUG                    SLOT                 NOTES
        mount      digits/torchaudio:hub   digits/system:mount  bind.1
        mount      digits/torchvision:hub  digits/system:mount  bind.1

   Both plugs share the same :samp:`bind.1` note,
   which implies they reference the same mount.


#. Any operation on one side automatically applies to the other.
   For example,
   after remounting :samp:`torchaudio:hub`,
   the information for :samp:`torchvision:hub` is updated as well:

   .. code-block:: console

      $ mkdir -p .cache/hub
      $ workshop remount digits/torchaudio:hub .cache/hub
      $ workshop info digits

        ...
        mounts:
          hub:
            host-source:      ~/digits/.cache/hub
            workshop-target:  /home/workshop/.cache/torch/hub


See also
--------

Explanation:

- :ref:`exp_plug_bindings`


Reference:

- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_definition`
