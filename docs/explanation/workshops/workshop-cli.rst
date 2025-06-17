:hide-toc:

.. _exp_workshop_cli:

workshop (CLI)
==============

.. @artefact workshop (CLI)
.. @artefact workshopd

|ws_markup| includes an eponymous command-line utility,
:command:`workshop`;
it is the daily go-to instrument for regular users,
with a set of commands that govern the entire life cycle of a
:ref:`workshop <exp_workshop>`.

.. note::

   The utility talks to the |ws_markup| daemon,
   :program:`workshopd`, via a REST API,
   so alternatives are possible and, in fact, encouraged.

There are several categories of commands that vary by their purpose:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 10 11 20

   * - Actions
     - Commands
     - What they do

   * - Create, update, delete
     - :command:`connect`,
       :command:`disconnect`,
       :command:`launch`,
       :command:`refresh`,
       :command:`remount`,
       :command:`remove`,
       :command:`sketch-sdk`
     - Control a workshop's existence;
       not to be confused with starting or stopping a workshop.

   * - Start, stop
     - :command:`start`,
       :command:`stop`
     - Begin and end the run-time life cycle of an existing workshop.

   * - Explore, troubleshoot
     - :command:`changes`,
       :command:`connections`,
       :command:`info`,
       :command:`list`,
       :command:`okay`,
       :command:`scripts`,
       :command:`sketches`,
       :command:`tasks`,
       :command:`warnings`
     - Enumerate workshops, list their details and recent activities.

   * - Utilize
     - :command:`exec`,
       :command:`shell`,
       :command:`run`
     - Run commands inside a workshop.


For an end-to-end example of putting these commands to use,
refer to the :ref:`tutorial <tutorial>`.


See also
--------

Reference:

- :ref:`ref_cli`
