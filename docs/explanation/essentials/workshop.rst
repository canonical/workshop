.. _exp_workshop:

Workshop
========

A *workshop*
(lowercase; not to be confused with |project_markup| itself)
is a container that is described in a definition file,
which is tied to a :ref:`project directory <exp_project>`.
Currently, these containers are hosted by
`LXD <https://documentation.ubuntu.com/lxd/en/latest/>`__,
but relying on this implementation detail isn't recommended.


.. _exp_workshop_def:

Workshop definition
-------------------

This is a file named :file:`.workshop.<NAME>.yaml`
that lists the base image of the workshop
and the specific components installed on top of it.
The definition serves as a single source of truth about the workshop.
It usually takes a few tries
to arrive at a configuration that suits your project,
so you can edit and update the workshop definition iteratively.

A simple definition may look like this:

.. code-block:: yaml
   :caption: .workshop.golang.yaml

   name: golang
   base: ubuntu@22.04
   sdks:
     go:
       channel: latest/stable

This specifies a *base* and an *SDK*.
A more complete definition would usually list multiple SDKs
that use different :ref:`interfaces <exp_interfaces>`,
packages and :ref:`hooks <exp_sdk_hooks>`.


.. _exp_workshop_base:

Base image
----------

The base is a supported OS image
that is used as the foundation of the workshop.
Currently, it can be either :samp:`ubuntu@20.04` or :samp:`ubuntu@22.04`.


See also
--------

Explanation:

- :ref:`project (concept) <exp_project>`
- :ref:`SDKs (concept) <exp_sdk>`