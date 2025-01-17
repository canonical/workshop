.. _ref_workshop_sketch-sdk:

workshop sketch-sdk
-------------------

Edit the sketch SDK and graft it onto the workshop.

.. rubric:: Usage

.. code-block:: console

   $ workshop sketch-sdk [--stash|--restore|--remove] [<WORKSHOP>] [flags]

.. rubric:: Description


This opens the 'sketch' SDK definition in the default text editor,
enabling rapid experiments and tweaks at the SDK level.

Saving the definition and exiting the editor causes a refresh,
which installs the configured 'sketch' SDK in the workshop.

The '--stash' and '--restore' options respectively stash the SDK,
reversing the changes, and quickly restore it to the workshop.
The '--remove' option removes the SDK permanently.

Notes:

- The 'sketch' SDK doesn't appear in the workshop definition
  and cannot include build-time data such as parts.

- In addition to hooks, the 'sketch' SDK can use interfaces,
  define plugs, slots, connections and bindings.

- You can partially refresh the workshop, targeting the 'sketch' SDK
  with the 'workshop refresh <WORKSHOP>/sketch' command.


.. rubric:: Examples


Edit the sketch SDK definition for the 'nimble' workshop
and apply it after saving by automatically refreshing the workshop:

.. code-block:: console

   $ workshop sketch-sdk nimble


The name is optional if the project has only one workshop:

.. code-block:: console

   $ workshop sketch-sdk


Stash the sketch SDK, temporarily reverting the changes in the workshop:

.. code-block:: console

   $ workshop sketch-sdk nimble --stash



.. rubric:: Flags


--remove

   Remove the sketch SDK from the workshop.


--restore

   Return the previously stashed SDK to the workshop.


--stash

   Stash the sketch SDK and remove it from the workshop.


