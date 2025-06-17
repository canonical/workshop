.. _how_forward_ports:

How to forward ports with tunneling
===================================

.. @artefact tunnel interface

Port-forwarding in |ws_markup| is done with the *tunnel interface*.
Tunnels pair a *plug* (the listening side) with a *slot* (the service side),
forwarding every connection that reaches the plug address to the slot address.
This how-to guide walks you through three common scenarios.


Exposing workshop services
--------------------------

This scenario was covered earlier in the
:ref:`tutorial section on interfaces <tut_interfaces>`.
In short, you add a tunnel slot to the SDK that runs the service
and a matching plug to the :samp:`system` SDK:

.. code-block:: yaml
   :caption: workshop.yaml

   sdks:
     - name: go
       slots:
         caddy:
           interface: tunnel
           endpoint: localhost:8080        # service in the workshop
     - name: system
       plugs:
         caddy:
           interface: tunnel
           endpoint: localhost:8080        # port on the host


.. note::

   |ws_markup| doesn't resolve hostnames, but supports the aliases
   :samp:`localhost`, :samp:`ip6-localhost`, and :samp:`ip6-loopback`.


Refresh the workshop and start the service,
so the host can reach it at :samp:`localhost:8080`:

.. code-block:: console

   $ workshop refresh


Note that port numbers can be different from each other,
subject to the regular low-port limitations.
Ensure the plug port is free before refreshing,
or the tunnel will fail to activate.


Sharing host services
---------------------

When a service runs on the host and code inside the workshop needs it,
create the tunnel the other way around:
a slot in the :samp:`system` SDK (the provider)
and a plug in the regular SDK (the consumer).

The example shares the host's PostgreSQL server
(:samp:`localhost:5432`) with MLflow in the workshop:

.. code-block:: yaml
   :caption: workshop.yaml

   sdks:
     - name: mlflow
       channel: latest/stable
       plugs:
         postgres:
           interface: tunnel
           endpoint: localhost:5432        # where MLflow will connect
     - name: system
       slots:
         postgres:
           interface: tunnel
           endpoint: localhost:5432        # host PostgreSQL server


Refresh the workshop to pick up the changes:

.. code-block:: console

   $ workshop refresh


One notable difference is that the
:ref:`connection validation policies <exp_tunnel_connection>`
are more strict when the slot is defined on the host,
so an extra command is needed to connect the plug to the slot:

.. code-block:: console

   $ workshop connect mlflow/mlflow:postgres mlflow/system:postgres


After this, |ws_markup| validates the endpoints and enables the connection.
MLflow can now reach PostgreSQL at :samp:`localhost:5432`.
The same pattern works for any host-side TCP- or UDP-based service.


Cross-protocol forwarding
-------------------------

Tunnels are not limited to identical protocols on both ends.
Unix domain sockets are often used for local-only daemons.
The tunnel interface lets you bridge them to TCP ports and vice versa.

Why do this?

- Avoid port clashes:
  Listen on a unique Unix path and publish it on an arbitrary TCP port.

- Expose a local service:
  Make a Unix-only daemon visible to tools that only speak TCP.

.. note::

   Only TCP and Unix domain sockets can be bridged across a tunnel.
   UDP is not compatible with Unix domain sockets.


Workshop Unix domain socket to host TCP port
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Suppose a gRPC service inside the workshop
listens on :file:`/run/grpc-service.sock` (Unix).
You want to reach it on the host at :samp:`localhost:18080`:

.. code-block:: yaml
   :caption: workshop.yaml

   #...
   sdks:
     - name: grpc-service
       slots:
         api:
           interface: tunnel
           endpoint: /run/grpc-service.sock # Unix domain socket in the workshop
     - name: system
       plugs:
         api:
           interface: tunnel
           endpoint: localhost:18080        # chosen TCP port on the host


After a refresh,
the service will be reachable from the host at :samp:`grpc://localhost:18080`:

.. code-block:: console

   $ workshop refresh
   $ workshop info

     ...
     sdks:
       system:
         tunnels:
           api:
             from:  127.0.0.1:18080/tcp
             to:    /run/grpc-service.sock
     ...

.. note::

   The tunnel interface expands :envvar:`$HOME` and :envvar:`$XDG_RUNTIME_DIR`
   in socket file paths automatically, but refuses other variables.
   Only user-writable locations are accepted for security reasons.


Host Unix domain socket to workshop TCP port
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Now let's invert the flow.
Share a host abstract socket (which exists only in the kernel, not on disk)
with code inside the workshop on TCP port :samp:`9000`.

.. tip::

   Abstract sockets avoid file system permissions and name collisions.
   They are written as :samp:`@name` (note the leading "@").


.. code-block:: yaml
   :caption: workshop.yaml

   #...
   sdks:
     - name: system
       slots:
         bus:
           interface: tunnel
           endpoint: '@bus'            # abstract socket on the host
     - name: client
       plugs:
         bus:
           interface: tunnel
           endpoint: localhost:9000    # TCP port inside workshop


After :command:`workshop refresh` and :command:`workshop connect`,
the code in the workshop can connect to :samp:`localhost:9000`,
and |ws_markup| forwards the traffic to the host's abstract socket :samp:`@bus`.


Troubleshooting
---------------

- TCP to Unix bridging is supported, while UDP to Unix is not.

- Ports below 1024 (privileged ports) may be rejected on the host side.

- Ensure the slot socket addresses exist and can be accessed by the |ws_markup| user;
  plug sockets are created by |ws_markup| so they shouldn't be already occupied.

- The tunnel won't activate if either side's endpoint is invalid;
  see error messages and :command:`workshop tasks` for hints.


See also
--------

Explanation:

- :ref:`exp_system_sdk`
- :ref:`exp_tunnel_interface`


Reference:

- :ref:`ref_tunnel_interface`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_tasks`
