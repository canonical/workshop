.. _how_use_caching_proxies:

.. meta::
   :description: How-to guide on making workshops download apt packages,
                 snaps, Go modules, and base images through caching proxies:
                 the snap options that point workshops and base image downloads
                 at them, pkgproxy as a worked example,
                 verifying the setup inside a workshop,
                 and how the settings interact with snapshots.

How to use caching proxies
==========================

.. @artefact workshop.apt.proxy
.. @artefact workshop.go.proxy
.. @artefact workshop.image.registry
.. @artefact workshop.snap.store.url

A workshop launched from scratch downloads its base image,
and its SDKs' setup hooks download packages with :program:`apt`,
snaps, and Go modules,
the same bytes on every machine and after every purge.
Caching proxies on the host or on the local network
download them once and serve them from their caches from then on,
which matters most on CI runners and new machines,
where no snapshot can be reused.

Each kind of download has its own snap option,
so any caching proxy that speaks the protocol works:

- :samp:`workshop.apt.proxy` for an HTTP proxy of :program:`apt`

- :samp:`workshop.snap.store.url` for a snap store API
  that :program:`snapd` uses instead of the Snap Store

- :samp:`workshop.go.proxy` for a :envvar:`GOPROXY` list
  of Go module proxies

- :samp:`workshop.image.registry` for an LXD image registry
  that base images come from


This guide uses `pkgproxy <https://gitlab.com/zygoon/pkgproxy>`_,
one caching proxy that serves all four on one port,
as a worked example.


Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- The caching proxies running on the host or on a machine the host can reach,
  listening on an address that workshops can reach.
  This guide uses pkgproxy on the host;
  with its default :samp:`:3142`, it listens on every address of the host.

- A host firewall that lets workshops connect to the proxies.
  With :program:`ufw`, for pkgproxy on the host:

  .. code-block:: console

     $ sudo ufw allow in on workshopbr0 to any port 3142 proto tcp

- To download base images through a registry:
  LXD 6.10 or later.


Point workshops at the proxies
------------------------------

Set the options for the proxies you have and restart the daemon.
For pkgproxy, which serves the snap store API at its root
and Go modules under :samp:`/go`:

.. code-block:: console

   $ sudo snap set workshop \
       workshop.apt.proxy=http://127.0.0.1:3142 \
       workshop.snap.store.url=http://127.0.0.1:3142 \
       workshop.go.proxy=http://127.0.0.1:3142/go,direct
   $ sudo snap restart workshop.workshopd

     Restarted.


The options are independent:
set only those for which you have a proxy.
:samp:`direct` at the end of the :envvar:`GOPROXY` list
lets :program:`go` download a module from its origin
when the proxy doesn't have it.

A proxy on the host itself can be named
with :samp:`localhost` or a loopback address, as above:
workshops reach it at the address of the host
on the :samp:`workshopbr0` network.
For a proxy on another machine, use its address or host name instead.

The settings apply to workshops launched from then on.
Workshops that already exist keep the configuration they were created with;
remove and launch one again to change it.


Launch and verify
-----------------

Launch a workshop as usual:

.. code-block:: console

   $ workshop launch <NAME>

     "<NAME>" launched


Because the settings are part of the workshop's first boot,
the first launch after changing them runs the SDKs' setup hooks again,
this time through the proxies.

Check that :program:`apt` uses its proxy:

.. code-block:: console

   $ workshop exec <NAME> -- apt-config dump Acquire::http::Proxy

     Acquire::http::Proxy "http://10.153.172.1:3142";


Check that :program:`snapd` does:

.. code-block:: console

   $ workshop exec <NAME> -- systemctl show --property=Environment snapd.service

     Environment=SNAPPY_FORCE_API_URL=http://10.153.172.1:3142


Check that :program:`go` does:

.. code-block:: console

   $ workshop exec <NAME> -- printenv GOPROXY

     http://10.153.172.1:3142/go,direct


The address is that of the host on :samp:`workshopbr0`, and differs between hosts.
The access log of the proxy shows the requests of the setup hooks.


Download base images through a registry
---------------------------------------

On LXD 6.10 and later,
LXD downloads the Ubuntu cloud images
that |ws_markup| launches workshops from
through an image registry.
pkgproxy mirrors these images
and sets up a registry for them:

.. code-block:: console

   $ pkgproxyctl setup lxd-remote | sh


Point |ws_markup| at the :samp:`pkgproxy-ubuntu` registry it creates,
a mirror of :samp:`https://cloud-images.ubuntu.com/releases`:

.. code-block:: console

   $ sudo snap set workshop workshop.image.registry=pkgproxy-ubuntu
   $ sudo snap restart workshop.workshopd

     Restarted.


The next launch that needs a base image LXD doesn't have yet
downloads it through the registry.
To check where LXD got it from,
find its fingerprint with :command:`lxc image list local: --project default`,
then show the image:

.. code-block:: console

   $ lxc image info local:<FINGERPRINT> --project default

     ...
     Cached: no
     Auto update: disabled
     Source:
         Image registry: pkgproxy-ubuntu
         Alias: <FINGERPRINT>
     ...


:samp:`Cached: no` means that LXD keeps the image until it's removed.


Stop using the proxies
----------------------

Unset the options and restart the daemon:

.. code-block:: console

   $ sudo snap unset workshop \
       workshop.apt.proxy workshop.snap.store.url workshop.go.proxy \
       workshop.image.registry
   $ sudo snap restart workshop.workshopd

     Restarted.


Workshops launched from then on download packages directly.
Workshops created while the options were set
still use the proxies until they're removed and launched again.


See also
--------

Reference:

* :ref:`ref_snap_options`
