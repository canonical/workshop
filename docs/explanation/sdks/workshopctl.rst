.. _exp_workshopctl:

SDK health reports with :program:`workshopctl`
==============================================

.. @artefact workshopd
.. @artefact workshopctl
.. @artefact SDK
.. @artefact SDK health

The :program:`workshopctl` tool allows an SDK
to talk to the :program:`workshopd` daemon,
giving SDK authors a way to manage health checks and report their health.
Using a model similar to `snapctl <https://snapcraft.io/docs/using-snapctl>`_,
it simplifies internal workshop communication,
helping both SDK authors and users.


Introduction
------------

:program:`workshopctl` is a CLI tool
that an SDK author can use with some :ref:`hooks <exp_sdk_hooks>`
to communicate with the :program:`workshopd` daemon.
Under the hood, :program:`workshopctl` uses a socket exposed by the daemon
to fit into the workshop environment.

This interaction between SDKs and the :program:`workshopd` daemon
focuses on health checks in post-launch or refresh operations.
The tool provides commands to report SDK health,
list workshops that use the SDK and get their details.


SDK health
----------

A primary function of :program:`workshopctl` is
to allow SDKs to report their health
using the :samp:`set-health` subcommand.
This command allows important SDK health information to be reported
after the workshop using the SDK has been launched or refreshed.

To use the command with :program:`workshopctl`,
you specify the mandatory health status.
If it's not :samp:`okay`,
you can also supply an error code with a user-friendly message
to provide further details.

.. @artefact SDK publisher

This command is essential for SDK publishers
to communicate the health status of their SDKs
within the workshop environment;
then :program:`workshopd` determines the overall health status of a workshop.


Workshop status
---------------

.. @artefact check-health
.. @artefact workshop status

The :samp:`check-health` hook is central to this,
as it tells the :program:`workshopd` daemon the health of the SDK
when workshop is launched or refreshed.
The status of a workshop, such as *Ready*, *Pending* or *Error*,
depends on the run-time results of the hook:

- *Ready* means success: the hook sets SDK health to :samp:`okay`
  and gracefully exits with a zero code.

- *Pending*: The hook sets the SDK health to :samp:`waiting`.
  This means it will be retried, one attempt per second.
  If the retries fail 10 times consecutively
  or if 5 seconds pass without :samp:`set-health` being invoked,
  the SDK health is changed to :samp:`error`.

- *Error*: the hook exits with a non-zero code
  or explicitly sets SDK health to :samp:`error`.


See also
--------

Explanation:

- :ref:`exp_sdk_hooks`


Reference:

- :ref:`ref_workshopctl`
