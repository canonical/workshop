.. _exp_sdk_parts:

.. meta::
   :description: Explanation of SDK parts in Workshop, detailing how they
                 modularize SDKs and manage dependencies through source, build,
                 and staging phases for improved maintenance and updates.

SDK parts
=========

.. @artefact SDK
.. @artefact SDK part

Parts provide a way to modularize the SDK and manage its dependencies,
ultimately making it easier to maintain and update
by separating its deployment into sourcing, building and staging phases.


Summary
-------

Parts can be thought of as the building blocks of an SDK.
Each part in the :ref:`definition <exp_sdk_definition>`
encapsulates a different aspect of the SDK
and focuses on a specific feature or resource;
these can be libraries, binaries, or configuration files.

A part defines a number of preset attributes and lifecycle stages in YAML;
|sdk_markup| executes these definitions stage by stage
and iteratively resolves any dependencies between parts.
Eventually, this results in a uniform SDK,
ready for publishing and installation;
such SDKs arrive to the users prebuilt,
allowing to factor out build activities from :ref:`SDK hooks <exp_sdk_hooks>`
that |ws_markup| executes inside the workshop at runtime.


Implementation notes
--------------------

Full disclosure: |sdk_markup| borrows the
`Craft Parts <https://github.com/canonical/craft-parts/>`_
mechanism from the upstream
`Craft Application <https://github.com/canonical/craft-application/>`_
project,
ultimately sharing it with such tools as
`Snapcraft <https://documentation.ubuntu.com/snapcraft/stable/>`_
and
`Charmcraft <https://documentation.ubuntu.com/charmcraft/stable/>`_,
so general approaches that work for any of those will apply here.

Aside from not yet allowing :samp:`stage-packages` and :samp:`stage-snaps`,
|sdk_markup| doesn't further limit or expand the parts functionality.
However, be aware of the requirements and limitations
that the upstream project places on what's available
for a given base, plugin, source and so on.

A detailed explanation is available in the corresponding Craft Parts
`documentation section
<https://documentation.ubuntu.com/craft-parts/latest/explanation/>`_.


See also
--------

Explanation:

- :ref:`exp_sdks`


Reference:

- :ref:`ref_sdk_parts`
