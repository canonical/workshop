.. _exp_content_interface:

Content interface
=================

Introduction
------------

The content interface
exposes host file system locations
to individual SDKs
by mounting them inside the workshop
that references these SDKs.


Content interface plug
----------------------

An essential element here is the content interface plug
that is declared in the SDK definition.

.. attention::

   An SDK definition, usually named :file:`sdkcraft.yaml`,
   is different from a
   :ref:`workshop definition <exp_workshop_def>`,
   usually named :file:`.workshop.<NAME>.yaml`;
   the former is used to build SDKs with `SDKcraft`_
   and isn't normally needed with |project_markup|,
   whereas the latter is a crucial element of daily |project_markup| activities.

   The following example is provided only to detail how content interface works.


A basic structure includes the name of the plug itself,
the interface (:samp:`content`)
and the intended target path inside the workshop:

.. code-block:: yaml
   :caption: sdkcraft.yaml

   name: go
   title: Go SDK
   base: ubuntu@20.04
   summary: The Go programming language
   description: |
     Go is an open source programming language that enables the production
     of simple, efficient and reliable software at scale.

   plugs:
     mod-cache:
       interface: content
       target: /home/workshop/go/pkg/mod


This definition creates a plug called :samp:`mod-cache`
that does the following:

- Sets its :samp:`interface` type to :samp:`content`,
  which makes it a content interface plug

- Points the :samp:`target` directory
  to :file:`/home/workshop/go/pkg/mod/`
  *inside the workshop*;
  a directory on the host system
  that |project_markup| designates at run-time
  will be mounted to it

Overall, the intent of this declaration is
to use a directory
(which |project_markup| automatically allocates for the slot)
for persisting the
`module cache <https://go.dev/ref/mod#module-cache>`__
in the host file system
when the workshop stops.


Content interface slot
----------------------

To let SDKs in a workshop access the host file system,
|project_markup| creates a content interface slot,
which multiple content interface plugs can then access.

.. note::

   Currently, content can only be exposed by |project_markup| itself,
   but can't be shared between two workshops directly.


When the SDK is installed
during :command:`workshop launch` and :command:`workshop refresh`
at run-time,
for each plug that targets the slot, |project_markup| checks the following:

- The plug can be installed.

- The plug can be auto-connected
  (for :samp:`content`, it's a yes).

- The :samp:`target` directory already exists in the workshop.

If the plug passes the checks,
it is connected
and a |project_markup|-created directory from the host file system
is mounted to the :samp:`target` directory inside the workshop.
That's where the module cache from our example will end up;
the best part is that it will be preserved between workshop operations such as
:command:`refresh`, :command:`start` and :command:`stop`,
so you can benefit from a pre-populated module cache without doing extra work.


See also
--------

Explanation:

- :ref:`SDK definition (concept) <exp_sdk_definition>`
- :ref:`SDK interface (concept) <exp_interfaces_plugs_slots>`
- :ref:`workshop base (concept) <exp_workshop_base>`
- :ref:`workshop definition (concept) <exp_workshop_def>`


Reference:

- :ref:`workshop refresh (command) <ref_workshop_refresh>`
- :ref:`workshop start (command) <ref_workshop_start>`
- :ref:`workshop stop (command) <ref_workshop_stop>`
