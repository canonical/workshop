Security
========

This is an overview of security considerations for |ws_markup| and |sdk_markup|.


Privileges
----------

.. @artefact workshopd
.. @artefact API

|ws_markup| has a client-server architecture;
its CLI, which is the contact surface for the users,
is confined as a snap and neither needs nor requires elevated privileges to run.
Instead, it uses a RESTful API to communicate with the :program:`workshopd` daemon,
which performs all the heavy lifting and does indeed run with elevated privileges.
The use of `LXD <https://documentation.ubuntu.com/lxd/en/latest/>`_
for implementation provides the benefits of a mature container technology.

|sdk_markup| is an instance of `craft-application
<https://github.com/canonical/craft-application/>`_,
built, installed and run as a snap;
it neither needs nor requires elevated privileges to work
and securely confines the SDK build process to a container.

.. @artefact SDK
.. @artefact SDK Store

Packaged SDKs are uploaded to the SDK Store.
Currently, it's implemented using `GCP
<https://console.cloud.google.com/storage/browser/sdk-store>`_,
so access is managed by `GCP IAM
<https://cloud.google.com/security/products/iam>`_.


Isolation
---------

Users can only access the workshops they have created;
these workshops have limited capabilities on the host.
To achieve this, LXD is used to add a level of confinement:
everything users do ends up in a `non-privileged container
<https://documentation.ubuntu.com/server/how-to/containers/lxd-containers/#uid-mappings-and-privileged-containers>`_
within a dedicated `project <https://documentation.ubuntu.com/lxd/en/latest/explanation/projects/>`_,
which separates workshops that belong to different users
and isolates them from each other and the host system.

By design, all SDKs in a workshop can access any data inside it,
but have limited capabilities on the host.
To achieve this, LXD is used to add a level of confinement:
everything |ws_markup| users do ends up in a `non-privileged container
<https://documentation.ubuntu.com/server/how-to/containers/lxd-containers/#uid-mappings-and-privileged-containers>`_
within a dedicated `project <https://documentation.ubuntu.com/lxd/en/latest/explanation/projects/>`_,
which separates workshops that belong to different users
and isolates them from each other and the host system.


Interfaces
----------

In |ws_markup|, the interface mechanism plays a role in maintaining security
by controlling access between the workshop's components and the host system;
the implementation is largely similar to :program:`snapd`'s
`interface manager <https://snapcraft.io/docs/interface-management>`__:

- Interfaces define and control what resources a workshop can use,
  ensuring that permissions are explicitly granted and limited in scope.

- They are used to explicitly provide access to resources
  such as files, the GPU or the SSH agent.

- SDKs in a workshop, or the workshop itself,
  must declare the interfaces and the connections they need.
  This limits the resources a workshop can access.

- Some interfaces, such as mounts, are connected automatically by default;
  others require manual approval by the user.
  All connections are subject to built-in validation policies.

.. @artefact SDK publisher

- The use of interfaces reflects the least privilege principle,
  allowing publishers and users to request only the necessary permissions,
  reducing the attack surface.


Risks
-----

Although safeguards are in place,
the security of a workshop or an SDK largely depends on how it's designed.
For instance, it is advisable not to store sensitive data within workshops.
Instead, use mounts to provide access to data only to the SDKs that require it.
Another example is avoiding the connection of sensitive interfaces,
such as the SSH agent, unless absolutely necessary.

You can use environment variables in |ws_markup| commands for access tokens
or the :ref:`SSH interface <exp_ssh_interface>` for transparent key-based access
to securely handle sensitive data in your SDKs.

The SDKs available in a workshop are sourced from the SDK Store
and are generally reliable at this stage of development.
However, if you are cautious about potential risks,
assume from the outset that no SDK is free from security concerns.


Supported versions
------------------

Use the latest releases of |ws_markup| and |sdk_markup| from GitHub;
older releases may have known bugs
or be incompatible with latest changes.


Reporting a vulnerability
-------------------------

As with other high-priority cases,
report concerns in our
`Mattermost channel <https://chat.canonical.com/canonical/channels/sdk>`__
or `GitHub issues <https://github.com/canonical/workshop/issues>`__.
