.. _how_git_workshops:

How to use workshops with Git
=============================

.. @artefact workshop (container)

Workshops are designed to be used in common development ecosystems,
which makes their encounter with Git almost inevitable.
Let's look at how you can integrate workshops into your repo.


Initialization
--------------

To start, place the workshop definition
in your repository:

.. code-block:: console

   $ git init original
   $ cd original/


.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: latest/stable


Next,
launch the workshop
and start working on your code:

.. @artefact workshop launch

.. code-block:: console

   $ workshop launch


.. code-block:: go
   :caption: main.go

   package main

   import "fmt"

   func main() {
       fmt.Println("hello, Workshop")
   }


Mind that any activity
that relies on the workshop's contents
should now occur inside the workshop:

.. @artefact workshop exec

.. code-block:: console

   $ git add . && git commit -m "initial commit"
   $ workshop exec dev go build -x main.go


.. @artefact project

However, the resulting artifacts are exposed in the project directory:

.. code-block:: console

   $ ./main

     hello, Workshop


They stay there even if you remove the workshop:

.. @artefact workshop remove

.. code-block:: console

   $ workshop remove
   $ ./main

     hello, Workshop


.. tip::

   If you do remove the workshop at this step of the guide,
   relaunch it before proceeding further:

   .. code-block:: console

      $ workshop launch

From here, you can do whatever you like with your repo,
because |ws_markup| handles
:ref:`moving projects around <how_move_projects>` quite well.

With your dependencies accounted for,
restoring your build system after cloning the repo elsewhere
is as simple as re-launching the workshop from a new
*project directory*.

But what if you need to maintain multiple branches
that require different versions of the same workshop?
A common solution is to clone the repo several times
to manually synchronize the copies when needed,
but this approach is prone to errors and overhead.
Let's build something better and...


Use worktrees
-------------

Let's add a Git feature that works well with workshops,
namely :literalref:`git worktree<https://git-scm.com/docs/git-worktree>`.

One of |ws_markup|'s goals is
to simplify toggling external dependencies
such as frameworks or OS versions.
Say you want to investigate a problem that occurs on an older OS version,
so you create a new worktree just for that:

.. code-block:: console

   $ git worktree add ../hotfix
   $ cd ../hotfix/


.. @artefact workshop base image

Instead of bothering with virtual machines,
update the definition
to change the base image:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 2,5

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: go
       channel: noble/stable


Next, launch the redefined workshop to work on the problem:

.. code-block:: console

   $ workshop launch
   $ # Hacking away until the problem is solved
   $ git commit -m "solve problem with hotfix"
   $ cd ../original/
   $ git merge hotfix


As with regular directories,
|ws_markup| works well with
:literalref:`git worktree move<https://git-scm.com/docs/git-worktree#_commands>`:

.. @artefact workshop list

.. code-block:: console

   $ git worktree move ../hotfix/ ../resolved/
   $ workshop list --global

     Project                 Workshop  Status  Notes
     /home/user/original     dev       Ready   -
     /home/user/resolved     dev       Ready   -


Similarly,
when it comes to clean-up,
remove all workshops
before running :samp:`git worktree remove`:

.. code-block:: console

   $ workshop remove --project ../resolved/
   $ git worktree remove ../resolved/


So using :command:`git worktree` reduces the effort on sync, stash, and pull,
while |ws_markup| allows you to hot-swap an entire OS
or another complex dependency
by going from one directory to another.


See also
--------

Explanation:

- :ref:`exp_base`
- :ref:`exp_projects`
- :ref:`exp_workshop_definition`


How-to guides:

- :ref:`how_move_projects`


Reference:

- :ref:`ref_workshop_launch`
- :ref:`ref_workshop_remove`
