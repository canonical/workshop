.. _release_notes:

.. meta::
   :description: Release notes and upgrade instructions for Workshop and
                 SDKcraft, including new features, bug fixes, and
                 version-specific guidance.

Release notes and upgrade instructions
======================================

Each version brings new features, bug fixes,
and occasionally backwards-incompatible changes.

Where necessary,
these release notes also include specific upgrade instructions for each version.
For additional guidance, see the
:ref:`general instructions on preparing for and performing an upgrade
<release_upgrade>`.


Releases
--------

.. toctree::
   :hidden:

   Workshop and SDKcraft 0.9.5 <v0.9.5>
   Workshop and SDKcraft 0.9.4 <v0.9.4>
   Workshop and SDKcraft 0.9.3 <v0.9.3>
   Workshop and SDKcraft 0.9.2 <v0.9.2>
   Workshop and SDKcraft 0.9.1 <v0.9.1>
   Workshop v0.9.0 <v0.9.0>


A complete |ws_markup| installation comprises two snaps:

- :program:`workshop` is designed for common users.
- :program:`sdkcraft` is intended for SDK publishers.


Both are available for :samp:`amd64` and :samp:`arm64`.
Starting with 0.9.1, |ws_markup| and |sdk_markup| share the same version number.


Latest version
~~~~~~~~~~~~~~

- :doc:`Workshop and SDKcraft 0.9.4 <v0.9.4>`


Previous versions
~~~~~~~~~~~~~~~~~

.. note::

   These versions are no longer supported.

- :doc:`Workshop and SDKcraft 0.9.3 <v0.9.3>`
- :doc:`Workshop and SDKcraft 0.9.2 <v0.9.2>`
- :doc:`Workshop and SDKcraft 0.9.1 <v0.9.1>`
- :doc:`Workshop v0.9.0 <v0.9.0>`
- `Workshop v0.1.30 <https://github.com/canonical/workshop/releases/tag/v0.1.30>`_
- `SDKcraft 0.1.14 <https://github.com/canonical/sdkcraft/releases/tag/0.1.14>`_


Release policy and schedule
---------------------------

Our release cadence is biweekly, aligned with our development methodology.
Releases follow the `semantic versioning <https://semver.org/>`_ scheme.


Long-term support
~~~~~~~~~~~~~~~~~

We only provide support for the latest versions of |ws_markup| and |sdk_markup|.
If you encounter issues with an older version,
we recommend upgrading to the latest release first;
see the next section for guidance.


.. _release_upgrade:

Upgrade instructions
--------------------

Refresh the snaps using the
`--classic <https://snapcraft.io/docs/install-modes/>`_ option:

.. code-block:: console

   $ sudo snap refresh --classic workshop
   $ sudo snap refresh --classic sdkcraft


For prerequisites and other details, see the `Installation
<https://github.com/canonical/workshop?tab=readme-ov-file#installation>`_
section on GitHub, or follow the :ref:`tut_get_started` tutorial section.
