.. _how_moving_projects:

How to move projects around
===========================

.. @artefact project

It may be unclear how workshops react to everyday operations
such as moving or copying a project directory.
Let's spend some time talking about different aspects of this.


Before launch
-------------

A workshop that you didn't launch
is just a definition file
that behaves like any good file should.
Things change *after* you run :command:`workshop launch`:

.. code-block:: yaml
   :caption: /home/user/old/workshop.yaml

   name: golang
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: latest/stable

.. @artefact workshop --project
.. @artefact workshop launch

.. code-block:: console

   $ workshop launch --project /home/user/old/


Moving a project
----------------

This is the simplest scenario.
Start in the same project directory where you launched the workshop:

.. @artefact workshop list

.. code-block:: console

   $ workshop list --global

     Project                 Workshop  Status  Notes
     /home/user/old          golang    Ready   -


Move the project directory and check the workshop:

.. code-block:: console

   $ mv /home/user/old/ /home/user/new/
   $ workshop list --global

     Project                 Workshop  Status  Notes
     /home/user/new          golang    Ready   -


|ws_markup| handles the project's move graciously
so the workshop here remains as you would expect;
there are no loose ends to pick up,
no paths to update in your definition file.

However,
this only ensures the safe transition of the workshop itself,
so it's up to you to update any paths external to |ws_markup|
that point to the project's previous location.


Copying a project
-----------------

Now let's copy the project directory.
Again, start with the workshop's location:

.. code-block:: console

   $ workshop list --global

     Project                 Workshop  Status  Notes
     /home/user/old          golang    Ready   -


Copy the project directory and check the workshops:

.. code-block:: console

   $ cp -r /home/user/old/ /home/user/new/

   $ workshop list --global

     Project                 Workshop  Status  Notes
     /home/user/old          golang    Ready   -


|ws_markup| won't launch the workshop in the new directory,
which is probably the sensible default here,
but what happens if you do it yourself?

.. code-block:: console

   $ workshop launch --project /home/user/new/
   $ workshop list --global

     Project                 Workshop  Status  Notes
     /home/user/old          golang    Ready   -
     /home/user/new          golang    Ready   -


Now, these are two independent workshops that happen to have the same name,
not a single workshop that is somehow shared by multiple project directories.

Again, it's up to you to update any paths external to |ws_markup|
that should point to your new project.


Removing a project
------------------

|ws_markup| doesn't handle file deletion automatically;
make sure you remove all workshops
before deleting the project directory:

.. @artefact workshop remove

.. code-block:: console

   $ workshop remove --project /home/user/old/
   $ rm -rf /home/user/old/


See also
--------

Explanation:

- :ref:`exp_projects`
- :ref:`exp_workshop_definition`


Reference:

- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_list`
- :ref:`ref_workshop_remove`
