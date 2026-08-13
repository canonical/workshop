.. _ref_workshop_sketch-sdk:


.. meta::
   :description: Reference documentation for the 'workshop sketch-sdk' command

workshop sketch-sdk
-------------------

.. @artefact workshop sketch-sdk

Customize a workshop.

.. rubric:: Usage

.. code-block:: console

   $ workshop sketch-sdk [<WORKSHOP>] [flags]

.. rubric:: Description

The command opens the sketch SDK template in the default text editor.
Add customizations by editing the template, then save and exit
the editor to apply the changes to the workshop.

The "--stash" and "--restore" options respectively stash the SDK,
reversing the changes, and quickly restore it to the workshop.

To make these customizations persistent,
run "workshop sketch-sdk --eject".
This saves the SDK definition under .workshop/ in the project directory,
so it can be committed to your repository.

The sketch SDK is intended for experiments and prototyping iterations.

Notes:

- You can only have one sketch SDK per workshop at a time.

- Run "workshop info" to list all SDKs currently installed
  in the workshop, including the sketch SDK if present.


.. rubric:: Examples


Edit the sketch SDK definition for the "nimble" workshop
and apply it after saving by automatically refreshing the workshop:

.. code-block:: console

   $ workshop sketch-sdk nimble


Save the sketch SDK for the "nimble" workshop
as a project SDK named "tools":

.. code-block:: console

   $ workshop sketch-sdk nimble --eject --name tools


Stash the sketch SDK, temporarily reverting the changes in the workshop:

.. code-block:: console

   $ workshop sketch-sdk nimble --stash



.. rubric:: Flags


--eject

   Promote the sketch SDK to an in-project SDK.


--name

   Name for the ejected SDK.


--remove

   Remove the sketch SDK from the workshop.


--restore

   Return the previously stashed SDK to the workshop.


--stash

   Stash the sketch SDK and remove it from the workshop.


--verbose

   Combine stdout and stderr output from hooks.




