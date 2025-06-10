.. _release_notes:

Release notes and upgrade instructions
======================================

This section provides an overview of new features, bug fixes,
and backwards-incompatible changes in each version.

Where necessary,
these release notes also include specific upgrade instructions for each version.
For additional guidance, see the
:ref:`general instructions on preparing for and performing an upgrade
<release_upgrade>`.


Releases
--------

We provide two binaries: |ws_markup| and |sdk_markup|.

- |ws_markup| (**AArch64** and **x86_64**) is designed for common users.
- |sdk_markup| (**x86_64** only) is intended for SDK publishers.


Currently, neither is publicly available,
but you can confidently use the pre-release versions.


|ws_markup|
~~~~~~~~~~~

Latest version:

- `Workshop v0.1.17 <https://github.com/canonical/workshop/releases/tag/v0.1.17>`_

Older versions:

- Workshop v0.1.16 didn't go public
- `Workshop v0.1.15 <https://github.com/canonical/workshop/releases/tag/v0.1.15>`_
- `Workshop v0.1.14 <https://github.com/canonical/workshop/releases/tag/v0.1.14>`_
- `Workshop v0.1.13 <https://github.com/canonical/workshop/releases/tag/v0.1.13>`_
- `Workshop v0.1.12 <https://github.com/canonical/workshop/releases/tag/v0.1.12>`_
- `Workshop v0.1.11 <https://github.com/canonical/workshop/releases/tag/v0.1.11>`_
- `Workshop v0.1.10 <https://github.com/canonical/workshop/releases/tag/v0.1.10>`_
- `Workshop v0.1.9 <https://github.com/canonical/workshop/releases/tag/v0.1.9>`_
- `Workshop v0.1.8 <https://github.com/canonical/workshop/releases/tag/v0.1.8>`_
- `Workshop v0.1.7 <https://github.com/canonical/workshop/releases/tag/v0.1.7>`_
- `Workshop v0.1.6 <https://github.com/canonical/workshop/releases/tag/v0.1.6>`_
- `Workshop v0.1.5 <https://github.com/canonical/workshop/releases/tag/v0.1.5>`_
- `Workshop v0.1.4 <https://github.com/canonical/workshop/releases/tag/v0.1.4>`_
- `Workshop v0.1.3 <https://github.com/canonical/workshop/releases/tag/v0.1.3>`_
- `Workshop v0.1.2 <https://github.com/canonical/workshop/releases/tag/v0.1.2>`_
- `Workshop v0.1.0 <https://github.com/canonical/workshop/releases/tag/v0.1.0>`_


|sdk_markup|
~~~~~~~~~~~~

Latest version:

- `SDKcraft v0.1.5 <https://github.com/canonical/sdkcraft/releases/tag/v0.1.5>`_

Older versions:

- `SDKcraft v0.1.4 <https://github.com/canonical/sdkcraft/releases/tag/v0.1.4>`_
- `SDKcraft v0.1.3 <https://github.com/canonical/sdkcraft/releases/tag/v0.1.3>`_
- `SDKcraft v0.1.2 <https://github.com/canonical/sdkcraft/releases/tag/v0.1.2>`_
- `SDKcraft v0.1.1 <https://github.com/canonical/sdkcraft/releases/tag/v0.1.1>`_
- `SDKcraft v0.1.0 <https://github.com/canonical/sdkcraft/releases/tag/v0.1.0>`_


Release policy and schedule
---------------------------

Our release cadence is biweekly, aligned with our development methodology.
Releases follow the `semantic versioning <https://semver.org/>`_ scheme.


.. _release_upgrade:

Upgrade instructions
--------------------

To upgrade, visit our `GitHub page
<https://github.com/canonical/workshop/releases>`_
to download and install the latest snap:

.. code-block:: console

   $ sudo snap install --dangerous --classic ./workshop_0.1.17_amd64.snap

Snaps are available for the :samp:`amd64` and :samp:`arm64` architectures.

For prerequisites and other details, see the `Getting Started
<https://github.com/canonical/workshop?tab=readme-ov-file#getting-started>`_
section on GitHub or follow the :ref:`tutorial`.
