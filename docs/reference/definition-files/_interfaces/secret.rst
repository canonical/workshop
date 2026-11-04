..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Secret interface
~~~~~~~~~~~~~~~~

.. @artefact secret interface

The secret interface delivers a credential from the host keyring
to processes in the workshop on request.

- Plug attributes: none.

- Plug name: any valid plug name.
  Processes in the workshop request the value as :samp:`{SDK}.{PLUG}`.

- Plug owner: any regular SDK; not the system SDK.

- Slot: the system SDK only, through slots added in the workshop definition.
  Other SDKs cannot declare secret slots.

A secret slot describes the host keyring item to look up, never its value.
It takes these attributes and no others:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`attributes` (required)
     - object
     - Item attributes to search for, as names mapped to string values.
       Must contain at least one attribute.
       A lookup succeeds only when exactly one item in the collection
       carries all of them.

   * - :samp:`collection` (optional)
     - string
     - Keyring collection to search.
       When omitted, the collection that the keyring's :samp:`default` alias
       points to is used, as if the value were :samp:`default`.
       Any other value is matched against collection labels
       and must identify exactly one collection;
       no match or multiple matches make the request fail.
       When given, the value can't be empty or blank.


.. code-block:: yaml
   :caption: workshop.yaml

   sdks:
     - name: system
       slots:
         demo-key:
           interface: secret
           attributes:
             service: secret-demo
             account: test
