..
   Single-sourced snippet. Included by workshop-definition.rst,
   sdk-definition.rst, and sdkcraft-definition.rst.
   Do not add a top-level label; the including page provides the anchor.

Tunnel interface
~~~~~~~~~~~~~~~~

.. @artefact tunnel interface

The tunnel interface forwards a network address or Unix domain socket.

Both tunnel plugs and tunnel slots take a single attribute:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 1 6

   * - Key
     - Value
     - Description

   * - :samp:`endpoint`
     - string
     - Network address or Unix domain socket that forms one end of the tunnel.
       Defaults to :samp:`localhost/tcp` for both plugs and slots.

The :samp:`endpoint` value follows this grammar:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 7

   * - Field
     - Format

   * - Endpoint
     - :samp:`<ADDRESS>/<PROTOCOL>` for network endpoints;
       may be shortened to :samp:`<ADDRESS>` or :samp:`<PROTOCOL>` alone.

       :samp:`<PATH>` or :samp:`@<STRING>` for Unix domain sockets.

   * - Address
     - :samp:`<HOST>:<PORT>`; may be shortened to :samp:`<HOST>` or :samp:`<PORT>`.

   * - Protocol
     - Either :samp:`tcp` or :samp:`udp`. Defaults to :samp:`tcp`.

   * - Host
     - An IPv4 or IPv6 address.
       When a port is supplied, IPv6 addresses must be enclosed in square brackets.

       Supported aliases: :samp:`localhost`, :samp:`ip6-localhost`, and :samp:`ip6-loopback`.
       Defaults to :samp:`localhost`.

   * - Port
     - A TCP or UDP port number (1-65535).
       May be omitted, but only on one side of a connection; both sides then use the same port.

       For security, tunnel plugs in the system SDK cannot use privileged ports (1-1023).

   * - Path
     - Absolute path to a Unix domain socket.

       :envvar:`$HOME` expands to the user's home directory
       and :envvar:`$XDG_RUNTIME_DIR` expands to the user runtime directory
       (typically :file:`/run/user/1000`).

       For security, tunnel plugs in the system SDK cannot listen on sockets outside these two directories.

   * - String
     - An abstract socket name.

Endpoints that start with :samp:`[` or :samp:`@` must be quoted in YAML:

.. code-block:: yaml

   endpoint: '[::1]:8080/tcp'
   endpoint: '@abstract.sock'
