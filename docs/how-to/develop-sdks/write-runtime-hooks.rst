.. _how_write_runtime_hooks:

.. meta::
   :description: Write the five SDK lifecycle hooks (setup-base, setup-project,
                 check-health, save-state, restore-state) so that an SDK
                 participates in the workshop launch and refresh lifecycle.

How to write runtime hooks
==========================

.. @artefact SDK hook
.. @artefact setup-base
.. @artefact setup-project
.. @artefact check-health
.. @artefact save-state
.. @artefact restore-state

This guide shows how to write each of the five
:ref:`SDK lifecycle hooks <exp_sdk_hooks>`,
with a synthesized SDK that exercises the contract differences:
which user the hook runs as,
which working directory it starts in,
and which environment variables it can rely on.

|ws_markup| runs each hook as a :program:`bash` script
with :samp:`errexit` and :samp:`pipefail` set,
so any non-zero exit aborts the hook.
Where it differs is the privilege, working directory, and extra environment.


Prerequisites
-------------

You need a working |sdk_markup| installation
and a workshop you can launch and refresh
on a host with |ws_markup| installed.
This guide assumes a synthesized SDK named :file:`dotfiles-sdk`,
scaffolded with :command:`sdkcraft init`,
as covered in :ref:`how_declare_plugs_slots`.


Where hooks live
----------------

|sdk_markup| picks up any executable file in the SDK's :file:`hooks/` directory
whose name matches one of the five hook names.
Hooks are not listed in :file:`sdkcraft.yaml`;
|sdk_markup| enumerates them automatically when packing.

A complete hook tree looks like this:

.. code-block:: console

   $ ls hooks/

     check-health
     restore-state
     save-state
     setup-base
     setup-project


Each file is a bash script; mark it executable
(:command:`sdkcraft init` already does this for the scaffolded ones).


Write setup-base
----------------

:samp:`setup-base` runs as :samp:`root`,
once per launch or refresh,
before the project directory is mounted
and before plugs and slots are connected.
The working directory is the SDK's own :file:`hooks/` directory.
Use it for system-wide preparation
that other SDKs in the workshop may want to rely on:

.. code-block:: shell
   :caption: hooks/setup-base

   cat <<PROFILE >/etc/profile.d/dotfiles.sh
   export DOTFILES_SDK="$SDK"
   PROFILE


:envvar:`$SDK` always points at the SDK installation directory in the workshop,
so hooks can reference files the SDK shipped
without hardcoding a path.


Write setup-project
-------------------

:samp:`setup-project` runs as the :samp:`workshop` user,
not root,
with the working directory set to :file:`/project/`.
It also has :envvar:`$HOME`, :envvar:`$XDG_RUNTIME_DIR`,
and :envvar:`$DBUS_SESSION_BUS_ADDRESS` available,
so it can touch the user's home tree
and talk to user-session services.

Use it for per-project initialization:

.. code-block:: shell
   :caption: hooks/setup-project

   id -u >"$HOME/.dotfiles-uid"
   install -m 0644 -t "$HOME" "$SDK/skel/.bash_aliases"


The hook runs after auto-connect has finished,
so any mounts the SDK plugged into are visible at this point,
as well as the project directory itself,
and the home directory is writable by the :samp:`workshop` user.


Write check-health
------------------

:samp:`check-health` runs as :samp:`root`
from the SDK's :file:`hooks/` directory.
It is meant to be quick:
|ws_markup| waits at most five seconds
for :command:`workshopctl set-health` to be called.
If the hook neither exits nor calls :samp:`set-health` within that window,
the SDK's health is set to :samp:`error`.

Call :command:`workshopctl set-health okay`
when everything is in order;
otherwise, set :samp:`error` with a short message:

.. code-block:: shell
   :caption: hooks/check-health

   if ! sudo -u workshop --login bash -c 'test -f "$HOME/.dotfiles-uid"'; then
     workshopctl set-health error "setup-project marker missing"
     exit 0
   fi
   workshopctl set-health okay


Because :samp:`check-health` runs as root,
use :command:`sudo -u workshop` whenever the check needs the workshop user's
shell, environment, or file ownership.


Persist state with save-state and restore-state
-----------------------------------------------

When a workshop refreshes an SDK to a new revision,
anything that lives outside a connected plug
disappears unless the SDK explicitly preserves it.
:samp:`save-state` and :samp:`restore-state`
solve that case.
Both hooks run as :samp:`root`
from the SDK's :file:`hooks/` directory
and have :envvar:`$SDK_STATE_DIR` available,
pointing at a directory that survives the refresh.

:samp:`save-state` runs from the *old* SDK revision
before the swap.
Write whatever needs to outlive the revision
into :envvar:`$SDK_STATE_DIR`:

.. code-block:: shell
   :caption: hooks/save-state

   if [ -f /home/workshop/.dotfiles-uid ]; then
     cp /home/workshop/.dotfiles-uid "$SDK_STATE_DIR/"
   fi


:samp:`restore-state` runs from the *new* SDK revision
after the swap and after :samp:`setup-project` has finished
for every SDK in the workshop:

.. code-block:: shell
   :caption: hooks/restore-state

   if [ -f "$SDK_STATE_DIR/.dotfiles-uid" ]; then
     install -m 0644 -o 1000 -g 1000 \
       "$SDK_STATE_DIR/.dotfiles-uid" \
       /home/workshop/.dotfiles-restored-uid
   fi


Keep the hook idempotent and tolerant of missing input,
since the new revision may be installed onto a workshop
that was originally launched without :samp:`save-state`.


Verify the hooks
----------------

Try the SDK in place:

.. code-block:: console

   $ sdkcraft try


Add the SDK to a workshop and launch it:

.. code-block:: console

   $ workshop launch dev


At this point :samp:`setup-base`, :samp:`setup-project`,
and :samp:`check-health` have all run.
Confirm:

.. code-block:: console

   $ workshop exec dev -- cat /etc/profile.d/dotfiles.sh

     export DOTFILES_SDK="/var/lib/workshop/sdk/dotfiles-sdk"

   $ workshop exec dev -- cat /home/workshop/.dotfiles-uid

     1000

   $ workshop info dev

     name: dev
     ...
     status: ready


:samp:`save-state` and :samp:`restore-state` only run
when :command:`workshop refresh` has work to do:
a new SDK revision to swap in,
an added or removed SDK,
or a change to the workshop definition.
A bare :command:`workshop refresh dev` against an unchanged workshop
is a no-op and skips every hook.

To exercise the state hooks,
bump the SDK to a new revision (for example, with :command:`sdkcraft pack`
followed by reinstalling it in the workshop)
or edit the workshop definition so the refresh has something to apply.
After the refresh,
:file:`.dotfiles-restored-uid` exists in the workshop user's home,
confirming that :samp:`save-state` wrote into :envvar:`$SDK_STATE_DIR`
and :samp:`restore-state` read it back.


See also
--------

Explanation:

- :ref:`exp_sdk_best_practices`
- :ref:`exp_sdk_hooks`


How-to guides:

- :ref:`how_configure_mount_ownership`
- :ref:`how_declare_plugs_slots`


Reference:

- :ref:`ref_sdk_hooks`
- :ref:`ref_sdk_state`
- :ref:`ref_workshopctl__cli`


Tutorial:

- :ref:`tut_craft_sdks`
