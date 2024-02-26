.. _ref_sdk_hooks:

SDK hooks
=========

|project_markup| supports the following life cycle hooks:

.. list-table::
   :header-rows: 1
   :width: 95
   :widths: 2 5 4

   * - Name
     - When it runs
     - What it does

   * - :samp:`check-health`
     - At :ref:`ref_workshop_launch`:
       after running :samp:`setup-base` hooks for *all* SDKs.
     
       At :ref:`ref_workshop_refresh`:
       after running :samp:`restore-state` hooks for *all* SDKs.

     - Reports the state of the SDK
       (*OK*, *waiting* or *error*)
       for |project_markup|
       to determine the overall state of the workshop.


   * - :samp:`restore-state`

     - At :ref:`ref_workshop_refresh`:
       after launching the new workshop
       and running the :samp:`setup-base` hook for the SDK.

     - Restores SDK-specific data from a |project_markup|-defined location.
       The hook itself comes from the *new* SDK version.


   * - :samp:`save-state`

     - At :ref:`ref_workshop_refresh`:
       before destroying the old workshop.

     - Preserves SDK-specific data at a |project_markup|-defined location.
       The hook itself comes from the *old* SDK version.


   * - :samp:`setup-base`

     - At :ref:`ref_workshop_launch`, :ref:`ref_workshop_refresh`:
       after unpacking the base image
       and starting the workshop,
       but before setting its status to *Ready*.

     - Configures the base image for the SDK to become operational.


See also
--------

Explanation:

- :ref:`SDK (concept) <exp_sdk>`
- :ref:`SDK state (concept) <exp_sdk_state>`
- :ref:`workshop base (concept) <exp_workshop_base>`
- :ref:`workshop definition (concept) <exp_workshop_def>`