.. _how_restrict_writes:

.. meta::
   :description: How-to guide on using the system SDK's AppArmor deny-write
                 policy to block writes to specific paths inside a workshop,
                 for example to stop an AI coding agent from touching
                 .git/hooks.

How to restrict writes with AppArmor
=====================================

.. @artefact apparmor deny-write

An AI coding agent, a misbehaving script, or a careless command
running inside a workshop can write anywhere its user can:
including places that should stay untouched,
such as :file:`.git/hooks/` in the project checkout.
|ws_markup| can enforce a hard, kernel-level block
on writes to specific paths using AppArmor,
independent of file permissions and :command:`sudo`.


Deny writes to a path
----------------------

Add an :samp:`apparmor` block to the :samp:`system` SDK entry
in :file:`workshop.yaml`,
listing the in-workshop paths to protect under :samp:`deny-write`:

.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 4-6

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: system
       apparmor:
         deny-write:
           - $PROJECT/.git/hooks

:samp:`$PROJECT` expands to :file:`/project`,
the mount point of the project checkout inside the workshop.
Every write, create, rename, or delete
under :file:`/project/.git/hooks/` (and the directory itself)
is now denied, no matter which user or process attempts it.

This setting only takes effect on a fresh instance.
Remove and relaunch the workshop to apply it:

.. @artefact workshop remove
.. @artefact workshop launch

.. code-block:: console

   $ workshop remove dev
   $ workshop launch dev


:command:`workshop refresh` does not apply AppArmor changes,
the same way it does not re-apply other instance-level settings
such as :samp:`raw.idmap` or :samp:`security.nesting`.


Confirm the policy
-------------------

:command:`workshop info` lists the resolved, protected paths
under the :samp:`system` SDK:

.. @artefact workshop info

.. code-block:: console
   :emphasize-lines: 5-6

   $ workshop info dev

     ...
     sdks:
       system:
         apparmor:
           deny-write:
             - /project/.git/hooks
     ...


From inside the workshop, writes under the protected path fail,
even with :command:`sudo`:

.. code-block:: console

   $ workshop exec dev -- sudo touch /project/.git/hooks/pre-commit

     touch: cannot touch '/project/.git/hooks/pre-commit': Permission denied


Restrictions
------------

- Only valid on the :samp:`system` SDK entry;
  any other SDK with an :samp:`apparmor` block fails validation
  at :command:`workshop launch` or :command:`workshop refresh`.
- Only supported on the default :samp:`lxd-container` runtime.
  It is rejected on :samp:`runtime: lxd-vm`,
  since AppArmor there confines the host-side virtual machine process,
  not the workshop's guest filesystem;
  see :ref:`how_use_virtual_machines` for that runtime's other trade-offs.
- Each path must be absolute, clean (no :samp:`..` segments),
  free of AppArmor glob characters (:samp:`* ? [ ] { } ^`),
  and a strict sub-path of a mount root:
  :file:`/project` itself cannot be denied, only something under it.


See also
--------

- :ref:`ref_workshop_definition_apparmor`
- :ref:`exp_system_sdk`
