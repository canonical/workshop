.. _ref_workshop_launch:

workshop launch
---------------

Construct one or many workshops using their definitions

Synopsis
~~~~~~~~


This command constructs the workshops listed as arguments by going over their
definitions and installing their components. For each workshop, it:

- Checks the workshop definition and identifies necessary actions

- Retrieves the required components, such as base and SDKs

- Runs SDK setup hooks to initialise the working state

- On success, ties the workshop to the project and starts it


If multiple workshops are listed and an error occurs,
the operation is aborted and no workshops are constructed.


Notes:

- Names listed as arguments must match respective 'name:' values in definitions

- To update an existing workshop, use 'workshop refresh' instead

- SDKs are installed in alphabetical order


.. code-block:: console

   workshop launch <WORKSHOP>... [flags]

Options
~~~~~~~
--no-wait

   Return the change ID, don't wait for the operation to finish


