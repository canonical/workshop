.. _ref_snap_options:

.. meta::
   :description: Reference of the Workshop snap configuration options
                 that configure the workshopd daemon:
                 the SDK Store, debug logging, the source of base images,
                 virtual machines, and the apt, snap, and Go proxies
                 of workshops.

Snap options
============

.. @artefact workshopd
.. @artefact workshop.apt.proxy
.. @artefact workshop.go.proxy
.. @artefact workshop.image.registry
.. @artefact workshop.snap.store.url

The |ws_markup| snap reads these configuration options
when :program:`workshopd` starts.
Set an option with :command:`snap set`,
then restart the daemon so that it reads the new value:

.. code-block:: console

   $ sudo snap set workshop <OPTION>=<VALUE>
   $ sudo snap restart workshop.workshopd


Unset an option the same way to return to its default:

.. code-block:: console

   $ sudo snap unset workshop <OPTION>
   $ sudo snap restart workshop.workshopd


:samp:`store.url`
-----------------

The URL of the SDK Store that |ws_markup| finds and downloads SDKs from.

- Default: :samp:`https://api.pkg.store`


:samp:`workshop.apt.proxy`
--------------------------

The HTTP proxy that :program:`apt` in new workshops downloads packages through,
as :samp:`http://<HOST>[:<PORT>]`;
see :ref:`how_use_caching_proxies`.

- Default: unset, :program:`apt` downloads packages directly


On the first boot of a new workshop,
before any SDK hook runs,
|ws_markup| sets the :samp:`Acquire::http::Proxy` option of :program:`apt`.
:program:`apt` uses an HTTP proxy for :samp:`http://` sources only,
so |ws_markup| also changes the Ubuntu archives in the sources of :program:`apt`
to use :samp:`http://` instead of :samp:`https://`;
:program:`apt` verifies what it downloads by the signatures of the archives.
See :ref:`ref_snap_options_proxies` for how workshops reach the proxy.


:samp:`workshop.debug`
----------------------

Set to :samp:`true` to make :program:`workshopd` log debug messages.

- Default: unset, no debug messages


:samp:`workshop.experimental-vms`
---------------------------------

Set to :samp:`1` to allow workshops with the :samp:`lxd-vm` runtime;
see :ref:`how_use_virtual_machines`.

- Default: unset, virtual machines are refused


:samp:`workshop.go.proxy`
-------------------------

The :envvar:`GOPROXY` list that :program:`go` in new workshops
downloads modules through;
see :ref:`how_use_caching_proxies`.
The entries are module proxy URLs,
as :samp:`http[s]://<HOST>[:<PORT>][/<PATH>]`,
and the keywords :samp:`direct` and :samp:`off`,
separated by commas or pipes,
as in :samp:`http://127.0.0.1:3142/go,direct`.

- Default: unset, :program:`go` uses its default proxy


On the first boot of a new workshop,
before any SDK hook runs,
|ws_markup| sets :envvar:`GOPROXY` in :file:`/etc/environment`,
which hooks, :command:`workshop exec` and SSH sessions read.
See :ref:`ref_snap_options_proxies` for how workshops reach the proxies.


:samp:`workshop.image.registry`
-------------------------------

The name of an LXD image registry that base images come from,
as :command:`lxc image registry list` shows it.
|ws_markup| looks up the alias of the base among the images of the registry,
as :command:`lxc image list --registry <NAME>` lists them,
and asks LXD to download the image through the registry,
as :command:`lxc image copy <NAME>:<ALIAS> local:` does.
It takes precedence over :samp:`workshop.image.server.url`.

- Default: unset, base images come from :samp:`workshop.image.server.url`

- Requires: LXD 6.10 or later,
  which has the :samp:`image_registries` API extension;
  launches fail with an error naming the extension on an older LXD


The registry must serve the base images under the same aliases
as :samp:`https://cloud-images.ubuntu.com/releases`,
such as :samp:`24.04`.
A registry that mirrors that server serves the same images,
so a base image already downloaded from one is not downloaded again from the other.


:samp:`workshop.image.server.url`
---------------------------------

The image server that base images come from,
as :samp:`<PROTOCOL>:<URL>`,
where :samp:`<PROTOCOL>` is :samp:`simplestreams` or :samp:`lxd`.
|ws_markup| looks up the alias of the base,
such as :samp:`24.04` for :samp:`ubuntu@24.04`,
on that server,
and asks LXD to download the image from it.

- Default: :samp:`simplestreams:https://cloud-images.ubuntu.com/releases`


LXD 6.10 and later download images only through an *image registry*,
and create one by themselves only for a few built-in servers,
such as :samp:`https://cloud-images.ubuntu.com/releases`.
With any other server, launches fail with
:samp:`No image registry found for image source server`;
use :samp:`workshop.image.registry` instead.


:samp:`workshop.snap.store.url`
-------------------------------

The URL of the snap store API that :program:`snapd` in new workshops
downloads snaps through,
as :samp:`http[s]://<HOST>[:<PORT>][/<PATH>]`,
such as a caching proxy of the Snap Store;
see :ref:`how_use_caching_proxies`.

- Default: unset, :program:`snapd` uses the Snap Store


On the first boot of a new workshop,
before any SDK hook runs,
|ws_markup| sets :envvar:`SNAPPY_FORCE_API_URL`
for :program:`snapd`,
with the drop-in
:file:`/etc/systemd/system/snapd.service.d/70-workshop-store.conf`,
and restarts it.
It also sets the variable in :file:`/etc/environment`
for :command:`snap download`.
See :ref:`ref_snap_options_proxies` for how workshops reach the store.


.. _ref_snap_options_proxies:

Proxies in workshops
--------------------

:samp:`workshop.apt.proxy`, :samp:`workshop.go.proxy`,
and :samp:`workshop.snap.store.url` are independent:
set any of them, or none.
None of them accepts user information, a query, or a fragment in a URL.

When the host of a URL is the host machine itself,
:samp:`localhost`, a loopback address or an unspecified address such as :samp:`0.0.0.0`,
workshops use the address of the host on the :samp:`workshopbr0` network instead.

The configuration is written once, when a workshop is created,
and |ws_markup| keeps it in the snapshots it takes after each SDK setup.
Snapshots taken with a different value of any of these options,
or with the option unset,
aren't used for new workshops,
so the first launch after a change sets up the SDKs again.
Workshops that already exist keep the configuration they were created with;
remove and launch one again to change it.
