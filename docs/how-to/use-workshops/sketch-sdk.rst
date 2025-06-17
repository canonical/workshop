.. _how_sketch:

How to customize workshops with sketch SDKs
===========================================

Suppose you built your workshop with a number of SDKs
only to realize it still lacks some functionality you need.
Naturally, you'd like to add that,
but can you align it with the way |ws_markup| operates?

.. @artefact SDK

Fortunately, |ws_markup| allows you to quickly draft a local SDK
and use it within your workshop. This process is called *sketching*.


Introduction
------------

We'll use the following scenario to demonstrate
how to iterate on an SDK to add missing functionality.

Suppose you're running the :samp:`dev` workshop from the :ref:`tutorial`:

.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: jammy/stable


This setup allows you to build and run Go code
while switching between language versions and base images.
However, in real-world usage, your project code would be stored in a repository,
and you'd likely use pre-commit checks and linters.
But what if existing SDKs in your workshop don't provide these checks?
Should you create and publish an SDK for your personal setup? Probably not.

In this guide, we'll use
`golangci-lint <https://github.com/golangci/golangci-lint>`_
and `shellcheck <https://www.shellcheck.net/>`_ as our tools of choice.
Let's explore how to integrate these utilities into your workshop
in a way that aligns with |ws_markup|.


Start sketching
---------------

Instead of manually installing tools using
:command:`workshop shell` or :command:`workshop exec`,
you can create a local SDK that automates these tasks with |ws_markup|.

.. @artefact SDK definition

Running :command:`workshop sketch-sdk`
opens a simplified version of an :ref:`SDK definition <exp_sdk_definition>`.
This defines all SDK components in a single file named :file:`sdk.yaml`:

.. @artefact workshop sketch-sdk

.. code-block:: console

   $ workshop sketch-sdk


The editor presents a minimal setup
with empty :samp:`hooks`, :samp:`plugs`, and :samp:`slots`:

.. code-block:: yaml
   :caption: sdk.yaml

   name: sketch

   hooks:
    # ...
   plugs:
    # ...
   slots:
    # ...


.. note::

   For more details on these components,
   see the :ref:`explanation <exp_index>` section.
   You may want to start with :ref:`exp_sdks` and :ref:`exp_interface`.


To install new software, locate the commented :samp:`setup-base`.
This hook runs when |ws_markup| launches or refreshes the SDK.
Uncomment :samp:`setup-base` and add the installation commands for our tools:

.. code-block:: yaml
   :caption: sdk.yaml

   name: sketch

   hooks:
     setup-base: |
       apt-get update
       apt-get install shellcheck
       snap install --classic golangci-lint


.. note::

   With |ws_markup|, you don't need to specify non-interactive flags like
   :option:`!-y` or :option:`!--no-install-recommends` with :program:`apt-get`;
   the environment handles this automatically.


Once you save and exit :file:`sdk.yaml`,
|ws_markup| refreshes the workshop, running the new hook:

.. code-block:: console

   Run hook "setup-base" for "sketch" SDK


If errors occur, you can :ref:`debug the installation process
<how_debug_issues_workshops>` as usual with :command:`workshop changes`,
:command:`workshop tasks`, and :command:`workshop refresh --continue` or
:command:`workshop refresh --abort`.
Mind that aborting the refresh does not revert your sketched changes,
so you can restart by running :command:`workshop sketch-sdk` again.

After the refresh, the output of :command:`workshop info` should look like this:

.. @artefact sketch SDK

.. code-block:: console

   sketch:
     tracking:   ~/.local/share/workshop/id/b5b0f128/dev/sdk/sketch/current
     installed:  2025-02-24  (x1)


The sketch SDK entry displays the last update time and its revision (:samp:`x1`).
The SDK is local, so :samp:`tracking` lists the SDK definition path on the host;
each edit with :command:`workshop sketch-sdk` increments the revision number.

At this point, you've created a functional, albeit simple, SDK in minutes.
For more complex needs, you can refine it iteratively.

.. note::

   You can only have one sketch SDK per workshop at a time;
   there's no way to create :samp:`sketch-foo`, :samp:`sketch-draft`,
   :samp:`sketch-final-final`, and so on.


Stash and restore
-----------------

You can temporarily stash the sketch SDK
to revert your workshop to its pre-sketching state:

.. code-block:: console

   $ workshop sketch-sdk --stash

.. important::

   Stashing does not delete the SDK,
   allowing you to restore it and continue working later.

   However, there's only one slot available for stashing.
   Running :command:`workshop sketch-sdk --stash` overwrites the existing stash,
   if any.
   Be cautious to avoid losing your changes.


To restore the stashed SDK:

.. code-block:: console

   $ workshop sketch-sdk --restore


Eject the SDK
-------------

.. @artefact in-project SDK
.. @artefact SDK Store

If you're satisfied with the sketch to a degree
where others may benefit from it,
you can either add it to your project
or publish it in the SDK Store.

The first step is to *eject* the SDK:

.. code-block:: console

   $ workshop sketch-sdk --eject

     "dev" sketch ejected to ".workshop/hello-workshop"
     To use it, add "project-hello-workshop" to the list of SDKs and run 'workshop refresh dev'


This moves the SDK definition files
into the :file:`.workshop/` subdirectory of the project
and removes the sketch from the running workshop.
|ws_markup| can pull SDKs from this directory,
bypassing the SDK Store.

If you'd rather publish the SDK for other projects to use,
|sdk_markup| can help.
For details, see the :ref:`how-to guide <how_sdkcraft>`.

The new SDK is named after the project directory by default;
use the :option:`!--name` option to change this:

.. code-block:: console

   $ workshop sketch-sdk --eject --name tools

     "dev" sketch ejected to ".workshop/tools"
     To use it, add "project-tools" to the list of SDKs and run 'workshop refresh dev'


Clean up
--------

To remove the sketch SDK permanently:

.. code-block:: console

   $ workshop sketch-sdk --remove


This deletes all changes introduced by the sketch.
Also, mind that :command:`workshop remove` removes the sketch SDK,
as you could expect,
including the stashed version.

To list all sketch SDKs in a project:

.. @artefact workshop sketches

.. code-block:: console

   $ workshop sketches


A project can have multiple workshops, remember;
hence the need to browse the respective sketches.


See also
--------

Explanation:

- :ref:`exp_dockerfile_vs_sdk`


Reference:

- :ref:`ref_sdk_definition`
- :ref:`ref_workshop_info`
- :ref:`ref_workshop_run`
- :ref:`ref_workshop_sketch-sdk`
- :ref:`ref_workshop_sketches`
