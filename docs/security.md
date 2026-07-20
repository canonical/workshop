# Security policy

This is an overview of security considerations for Workshop and SDKcraft.

## Privileges

Workshop has a client-server architecture; its CLI, which is the contact surface
for the users, is confined as a snap and neither needs nor requires elevated
privileges to run. Instead, it uses a RESTful API to communicate with the
`workshopd` daemon, which performs all the heavy lifting and does indeed run
with elevated privileges. The use of
[LXD](https://documentation.ubuntu.com/lxd/latest/) for implementation provides
the benefits of a mature container technology.

SDKcraft is an instance of
[`craft-application`](https://github.com/canonical/craft-application/), built,
installed, and run as a snap; it neither needs nor requires elevated privileges
to work and securely confines the SDK build process to a container.

Packaged SDKs are uploaded to the SDK Store.

## Isolation

Users can only access the workshops they have created; these workshops have
limited capabilities on the host. To achieve this, LXD is used to add a level of
confinement: everything users do ends up in a [nonprivileged
container](https://ubuntu.com/server/docs/how-to/containers/lxd-containers/)
within a dedicated
[project](https://documentation.ubuntu.com/lxd/latest/explanation/projects/),
which separates workshops that belong to different users and isolates them from
each other and the host system.

By design, all SDKs in a workshop can access any data inside it, but have
limited capabilities on the host, due to the confinement of the workshop.

## Interfaces

In Workshop, the interface mechanism plays a role in maintaining security by
controlling access between the workshop's components and the host system; the
implementation is largely similar to `snapd`'s [interface
manager](https://snapcraft.io/docs/interface-management/):

* Interfaces define and control what resources a workshop can use, ensuring that
  permissions are explicitly granted and limited in scope.
* They are used to explicitly provide access to resources such as files, the
  GPU, or the SSH agent.
* SDKs in a workshop, or the workshop itself, must declare the interfaces and
  the connections they need. This limits the resources a workshop can access.
* Some interfaces, such as mounts, are connected automatically by default;
  others require manual approval by the user. All connections are subject to
  built-in validation policies.
* The use of interfaces reflects the least privilege principle, allowing
  publishers and users to request only the necessary permissions, reducing the
  attack surface.

## Risks

Although safeguards are in place, the security of a workshop or an SDK largely
depends on how it's designed. For instance, it is advisable not to store
sensitive data within workshops. Instead, use mounts to provide access to data
only to the SDKs that require it. Another example is avoiding the connection of
sensitive interfaces, such as the SSH agent, unless absolutely necessary.

You can use environment variables in Workshop commands for access tokens or the
\:ref:`SSH interface <exp_ssh_interface>` for transparent key-based access to
securely handle sensitive data in your SDKs.

The SDKs available in a workshop are sourced from the SDK Store and are
generally reliable at this stage of development. However, if you are cautious
about potential risks, assume from the outset that no SDK is free from security
concerns.

## Supported versions

Use the latest releases of Workshop and SDKcraft from GitHub; older releases may
have known bugs or be incompatible with latest changes.

## Reporting a vulnerability

The easiest way to report a security issue is through GitHub, filing a private
security report with a description of the issue, affected versions, the steps to
reproduce the issue, and, if known, ways of mitigating it. See [Privately
reporting a security
vulnerability](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/privately-reporting-a-security-vulnerability)
for instructions.

Our GitHub admins will be notified of the issue and will work with you to
determine whether the issue qualifies as a security issue and, if so, in which
component. We will then handle figuring out a fix, getting a CVE assigned, and
coordinating the release of the fix.

The [Ubuntu Security disclosure and embargo
policy](https://ubuntu.com/security/disclosure-policy) contains more information
about what you can expect when you contact us and what we expect from you.

In lower-priority cases that do not affect security, you may report your
concerns in [GitHub issues](https://github.com/canonical/workshop/issues).

## Cryptography

Transport encryption
- CLI ↔ daemon: local Unix domain socket (no TLS required).
- Outbound traffic: HTTPS/TLS for simplestreams [image downloads](https://cloud-images.ubuntu.com/releases/)
and public LXD remotes via the Go TLS stack (through the LXD client).
- Mutual TLS (public LXD remotes): enable by supplying X.509 materials 
in `/var/lib/workshop/tls` (`server.crt`, `client.crt`, `client.key`, `client.ca`).

Internal cryptography
- TLS stack: Go's `crypto/tls` and `crypto/x509` (via the Canonical LXD Go client), 
using TLS 1.2/1.3 with Go's secure default cipher suites (ECDHE with AES‑GCM/ChaCha20‑Poly1305) 
and the system trust store by default.
- Randomness: `crypto/rand` for non‑guessable identifiers (e.g., 4‑byte project IDs, 8‑byte layer suffixes);
these values are not used for access control.

User‑exposed crypto and providers
- SSH agent interface: forwards the host's `ssh-agent` into the workshop via an LXD proxy device, 
allowing tools inside the container to authenticate without copying private keys.
- SSH host trust: a per-user Ed25519 certificate authority signs a host certificate for every
workshop and a user certificate for connecting to them, so SSH clients trust any CA-signed `*.wp`
host key without a manual host-key prompt (the trust covers the `*.wp` namespace, not an
individual hostname). Private keys are stored with owner-only permissions (`0600`), the CA key
owned by the daemon and the user key by the target host user.
- Algorithms: follow host OpenSSH (commonly Ed25519, ECDSA P‑256/P‑384/P‑521, RSA 2048/3072/4096).
- Providers: Go standard library (`crypto/tls`, `crypto/x509`, `crypto/rand`), 
Canonical LXD Go client (TLS handling), system CA store,
and OpenSSH packages from Ubuntu.
