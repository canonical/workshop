.. _ref_workshop_warnings:

workshop warnings
-----------------

List warnings.

.. rubric:: Usage

.. code-block:: console

   $ workshop warnings [flags]

.. rubric:: Description


This command lists the warnings that were reported to the system.

All warnings listed by 'workshop warnings'
can be acknowledged with the 'workshop okay' command.
Acknowledged warnings aren't listed by 'workshop warnings'
unless they occur again after their cooldown period has elapsed
or the '--all' option is used.

Also, warnings expire automatically; expired warnings are not listed.


.. rubric:: Examples


List the globally registered warnings across all workshops:

.. code-block:: console

   $ workshop warnings



.. rubric:: Flags


--abs-time

   Use absolute times in RFC 3339 format.
   By default, relative times are used up to 60 days, then YYYY-MM-DD.


--all

   Show all warnings, including the acknowledged ones.


--unicode

   Use Unicode characters to improve legibility (auto|never|always).
   By default, Unicode is used only if the output supports it.


--verbose

   Show more information per each warning.


