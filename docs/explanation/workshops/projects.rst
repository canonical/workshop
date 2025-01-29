:hide-toc:

.. _exp_projects:

Projects
========

.. @artefact project
.. @artefact project workshops
.. @artefact workshop definition
.. @artefact workshop .lock

Technically, a project is a directory
containing one workshop definition or more.

To initialise a directory as a project,
create a
:ref:`workshop definition <exp_workshop_definition>`
in it
and run :command:`workshop launch`.
Launching a workshop from a project
establishes the relationship between the two
that's required to actually start a workshop.
This is achieved with a hidden :file:`.lock` file,
which must remain in the project directory
and must not be copied or stored externally, e.g. in a repository.

You can store workshop definitions in two ways:

.. @artefact workshop name

- If you use a single workshop in the project,
  store its definition in the project directory as :file:`workshop.yaml`.
  This allows you to omit the workshop name in :ref:`the CLI <exp_workshop_cli>`.

- If your project involves multiple workshops,
  store their definitions in files with the same name as the workshops
  under the :file:`.workshop/` subdirectory of the project directory:

  .. code-block:: none

     .workshop/foo.yaml
     .workshop/bar.yaml

  When multiple workshop definitions are present,
  you can't omit the workshop name in commands.


When a workshop is then started with :command:`workshop start`,
the project directory is mounted to it as :file:`/project/`;
conversely, the :command:`workshop stop` command unmounts it.

.. @artefact workshop --project

.. note::

   There are more workshop CLI commands;
   some have a :option:`!--project` option
   that accepts a pathname to use as the project directory.

External changes to the project are tracked by the |ws_markup| daemon.
Thus, if the project is moved or copied,
all workshops that reference it are updated,
so you can continue working without interruption.

If the project is deleted by external means
without first removing its workshops,
any workshops that reference it
enter the *Error* state;
the only command applicable to them is :command:`workshop remove`.


See also
--------

Explanation:

- :ref:`exp_workshop`
- :ref:`exp_workshop_definition`


How-to guides:

- :ref:`how_moving_projects`


Reference:

- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_remove`
- :ref:`ref_workshop_start`
- :ref:`ref_workshop_stop`
