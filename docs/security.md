# Security policy

This is an overview of security considerations for Workshop and SDKcraft.

(security_privileges)=

## Privileges

Workshop has a client-server architecture; its CLI, which is the contact surface
for the users, is confined as a snap and neither needs nor requires elevated
privileges to run. Instead, it uses a RESTful API to communicate with the
`workshopd` daemon, which performs all the heavy lifting and does indeed run
with elevated privileges. The use of
[LXD](https://canonical.com/lxd/docs/latest/) for implementation provides
the benefits of a mature container technology.

SDKcraft is an instance of
[`craft-application`](https://github.com/canonical/craft-application/), built,
installed, and run as a snap; it neither needs nor requires elevated privileges
to work and securely confines the SDK build process to a container.

Packaged SDKs are uploaded to the SDK Store.

(security_isolation)=

## Isolation

Users can only access the workshops they have created; these workshops have
limited capabilities on the host. To achieve this, LXD is used to add a level of
confinement: everything users do ends up in a [nonprivileged
container](https://ubuntu.com/server/docs/how-to/containers/lxd-containers/)
within a dedicated
[project](https://canonical.com/lxd/docs/latest/explanation/projects/),
which separates workshops that belong to different users and isolates them from
each other and the host system. The exceptions are the project directory, which
every workshop in the project mounts read-write, and the host resources you
connect through interfaces.

By design, all SDKs in a workshop can access any data inside it, because
everything in the workshop runs as the same `workshop` user or as `root`; their
capabilities on the host are limited by the confinement of the workshop.

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

(security_risks)=

## Risks

Although safeguards are in place, the security of a workshop or an SDK largely
depends on how it's designed. For instance, it is advisable not to store
sensitive data within workshops. Instead, use mounts to provide access to data
only to the SDKs that require it. Another example is avoiding the connection of
sensitive interfaces, such as the SSH agent, unless absolutely necessary.

You can use environment variables in Workshop commands for access tokens or the
[SSH interface](https://ubuntu.com/workshop/docs/explanation/interfaces/ssh-interface/)
for transparent key-based access to securely handle sensitive data in your SDKs.

The SDKs available in a workshop are sourced from the SDK Store and are
generally reliable at this stage of development. However, if you are cautious
about potential risks, assume from the outset that no SDK is free from security
concerns.

(security_coding_agents)=

## Coding agents

A coding agent that runs in a workshop can do anything the workshop allows, and
permission modes that turn off the agent's approval prompts remove its own
safeguards. The workshop doesn't replace those safeguards; it only limits the
agent to the boundaries below, so weigh them before giving an agent that much
autonomy:

* **The project directory is writable.** Workshop mounts the entire project
  directory at `/project` read-write, including its `.git` directory, and files
  created inside the workshop belong to your host user. Git runs hooks from
  `.git/hooks`, and settings in `.git/config` can name commands for Git to run,
  so anything an agent writes there runs on your host the next time you use Git
  in the project. For an agent that works unattended, use a separate clone of
  the repository rather than your working copy, and review its changes before
  you bring them back.
* **Subdirectories and worktrees don't narrow the mount.** The workshop sees the
  whole project directory, whichever subdirectory the agent starts in. A Git
  worktree is a separate project with a mount of its own, but its `.git` file
  points to the main repository outside that mount, so Git commands fail inside
  the workshop; run them on the host.
* **Everything runs as one user.** Commands, actions, and agents run as the
  `workshop` user, which can use `sudo` without a password, and SDK hooks run
  as `root`.
  An agent can read and change anything in the workshop, including other SDKs'
  files and the credentials they store.
* **Outbound network access is open.** Workshop doesn't filter outgoing traffic,
  so an agent can reach any host that the workshop's network can reach and send
  project data there.
* **Connected interfaces extend the reach.** Avoid connecting these interfaces
  to a workshop where an autonomous agent runs, or disconnect them before the
  agent starts:
  * The
    [SSH interface](https://ubuntu.com/workshop/docs/explanation/interfaces/ssh-interface/)
    lets any process in the workshop authenticate with the identities in your
    host's SSH agent while it's connected.
  * [Mounts](https://ubuntu.com/workshop/docs/explanation/interfaces/mount-interface/)
    of host directories are writable unless the plug sets `read-only`; mounting
    an agent's configuration directory from your home exposes its credentials
    and settings to everything in the workshop.
  * The
    [desktop interface](https://ubuntu.com/workshop/docs/explanation/interfaces/desktop-interface/)
    shares your graphical session with the workshop.
  * [Tunnels](https://ubuntu.com/workshop/docs/explanation/interfaces/tunnel-interface/)
    let the workshop reach network services on your host.
  * The
    [camera](https://ubuntu.com/workshop/docs/explanation/interfaces/camera-interface/)
    and
    [custom device](https://ubuntu.com/workshop/docs/explanation/interfaces/custom-device-interface/)
    interfaces pass host devices into the workshop.
* **A virtual machine changes the kernel, not the rest.** The experimental
  virtual machine runtime gives a workshop its own kernel, but the project
  directory is still mounted read-write, and outbound network access is still
  open.

## Supported versions

Use the latest releases of Workshop and SDKcraft from GitHub; older releases may
have known bugs or be incompatible with latest changes.

(security_reporting)=

## Reporting a vulnerability

The easiest way to report a security issue is through GitHub, filing a private
security report with a description of the issue, affected versions, the steps to
reproduce the issue, and, if known, ways of mitigating it. See [Privately
reporting a security
vulnerability](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/report-privately)
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
workshop and a user certificate for connecting to them, so SSH clients trust any CA-signed
host key without a manual host-key prompt.
Private keys are stored with owner-only permissions (`0600`), the CA key
owned by the daemon and the user key by the target host user.
- Algorithms: follow host OpenSSH (commonly Ed25519, ECDSA P‑256/P‑384/P‑521, RSA 2048/3072/4096).
- Providers: Go standard library (`crypto/tls`, `crypto/x509`, `crypto/rand`), 
Canonical LXD Go client (TLS handling), system CA store,
and OpenSSH packages from Ubuntu.
