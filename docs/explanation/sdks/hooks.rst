:hide-toc:

.. _exp_sdk_hooks:

SDK hooks
=========

.. @artefact SDK
.. @artefact SDK hook
.. @artefact check-health
.. @artefact restore-state
.. @artefact save-state
.. @artefact setup-base

|ws_markup| and |sdk_markup| enable optional life cycle *hooks*
that control and extend the behaviour of an SDK.

Each hook is a shell script
that performs SDK-specific, domain-oriented actions in the workshop
at a particular life cycle stage
to ensure that the SDK stays functional.
Specific examples include :samp:`setup-base`,
:samp:`save-state` and :samp:`restore-state`.

When you define an SDK,
its hooks should be placed in the :file:`hooks/` subdirectory
next to the :ref:`definition <exp_sdk_definition>`;
|sdk_markup| validates and packages them along with the :file:`.yaml` file.


See also
--------

Explanation:

- :ref:`exp_sdk`


Reference:

- :ref:`ref_sdk_hooks`
