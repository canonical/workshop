.. _exp_tunnel_interface:

Tunnel interface
================

.. @artefact tunnel interface

The tunnel interface
enables workshops to share network services with the host system,
and vice versa.
It supports connections over TCP, UDP, and Unix domain sockets.

SDKs advertise their services using tunnel interface slots.
For example, if an SDK installs and advertises a web app,
users can access the app from their usual browser
after creating a tunnel from the host system to the workshop.

SDKs request access to services using tunnel interface plugs.
Some services have dedicated interfaces
(e.g. the :ref:`SSH interface <exp_ssh_interface>`),
which should be used instead.


.. _exp_tunnel_plug:

Tunnel interface plug
---------------------

Most SDKs declare tunnel interface plugs in their SDK definitions,
but the :ref:`system SDK <exp_system_sdk>` has none by default,
so system SDK plugs must be declared in the workshop definition.

A basic structure would include the name of the plug,
the interface (:samp:`tunnel`),
and, optionally, an address (:samp:`endpoint`).

Plugs designate addresses that clients can connect to.
Regular SDKs are used for clients inside the workshop.
The system SDK is used for clients from the host system.


.. _exp_tunnel_slot:

Tunnel interface slot
---------------------

Most SDKs declare tunnel interface slots in their SDK definitions,
but the :ref:`system SDK <exp_system_sdk>` has none by default,
so system SDK slots must be declared in the workshop definition.

A basic structure would include the name of the slot,
the interface (:samp:`tunnel`),
and, optionally, an address (:samp:`endpoint`).

Slots designate an address that a service can listen on.
Regular SDKs should make this service available inside the workshop.
The system SDK relies on the user to run this service on the host.


.. _exp_tunnel_connection:

Connection
----------

The interface is connected automatically at launch or refresh,
provided that:

- The plug is declared in the system SDK

- The slot is declared in a regular SDK

- The plug listens on :samp:`localhost` or a Unix domain socket

- The plug can be matched to the slot by its name,
  or via a :samp:`connections` entry in the :ref:`definition <exp_workshop_definition>`,
  both subject to |ws_markup|'s
  :ref:`validation rules <exp_interfaces_validation>`.


Otherwise, it isn't connected automatically,
for security reasons.
The :command:`workshop connect` and :command:`workshop disconnect` commands
can be invoked after the workshop has started:

.. @artefact workshop connect
.. @artefact workshop disconnect

.. code-block:: console

   $ workshop connect ws/client-sdk:shared
   $ workshop disconnect ws/client-sdk:shared
   $ workshop connect ws/system:app ws/service-sdk:app
   $ workshop disconnect ws/service-sdk:app


Establishing a tunnel connection means
that |ws_markup| will listen on the plug address,
forwarding incoming network connections to the slot address.

When a system SDK plug is connected to a regular SDK slot,
clients on the host can access services inside the workshop:

.. mermaid::
   :alt: Exposing SDK services to the host system
   :caption: Exposing SDK services to the host system
   :align: center
   :config: {"theme":"neutral"}

   flowchart LR
     subgraph Host
       Client --> Plug

       subgraph system[System SDK]
         Plug
       end
     end

     Plug -- Tunnel --> Slot

     subgraph Workshop
       subgraph regular[Regular SDK]
         Slot --> Service
       end
     end


When a regular SDK plug is connected to a system SDK slot,
clients in the workshop can access services on the host:

.. mermaid::
   :alt: Sharing system services with a workshop
   :caption: Sharing system services with a workshop
   :align: center
   :config: {"theme":"neutral"}

   flowchart RL
     subgraph Workshop
       subgraph regular[Regular SDK]
         Client --> Plug
       end
     end

     Plug -- Tunnel --> Slot

     subgraph Host
       subgraph system[System SDK]
         Slot
       end

       Slot --> Service
     end


|ws_markup| doesn't support connections within the system SDK
or between regular SDKs.
In these cases clients can connect to services directly,
without the need for a tunnel.

To check if a plug or slot is connected:

.. @artefact workshop connections

.. code-block:: console

   $ workshop connections --all

     Interface  Plug                  Slot                Notes
     ...
     tunnel     ws/client-sdk:shared  ws/system:shared    manual
     tunnel     ws/system:app         ws/service-sdk:app  manual


This means that :samp:`client-sdk` can access
the :samp:`shared` service running on the host,
and the host can access the :samp:`app` service
provided by :samp:`service-sdk`.

.. @artefact workshop info

.. code-block:: console

   $ workshop info dev

     name:     dev
     base:     ubuntu@22.04
     project:  /home/user/workshop/dev
     status:   ready
     notes:    -
     sdks:
       system:
         tunnels:
           app:
             from:  0.0.0.0:8081/tcp
             to:    127.0.0.1:8080/tcp
       client-sdk:
         tracking:   latest/stable
         installed:  2024-03-02  (1)
         tunnels:
           shared:
             from:  [::1]:1080/tcp
             to:    127.0.0.1:18080/tcp
       service-sdk:
         tracking:   latest/edge
         installed:  2025-06-07  (2)


See also
--------

Explanation:

- :ref:`exp_interfaces`
- :ref:`exp_plugs_slots`
- :ref:`exp_sdk_definition`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_tunnel_interface`
- :ref:`ref_workshop_connect`
- :ref:`ref_workshop_connections`
- :ref:`ref_workshop_disconnect`
- :ref:`ref_workshop_info`
- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_refresh`
