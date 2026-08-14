:relatedlinks: [Diátaxis](https://diataxis.fr/), [Documentation starter pack](https://documentation.ubuntu.com/sphinx-stack/latest/), [reST style guide](https://documentation.ubuntu.com/sphinx-stack/latest/reference/style-guide/)

.. _contributing_documentation:
.. _contributing_doc:

.. meta::
   :description: Make a documentation change to Workshop. Set up, write, build,
                 and review documentation within the project's standard workflow.

Contribute to this documentation
================================

|ws_markup|'s documentation lives in the :file:`docs/` directory.
A documentation change follows the same arc as a code change,
from a local preview through to a merged pull request.


Set up your work environment
----------------------------

All documentation resides in the :file:`docs/` directory.
To build and serve it at :samp:`127.0.0.1:8000`:

.. code-block:: console

   $ workshop launch dev
   $ workshop run dev docs-run

The workshop is the same one code contributions use;
see :ref:`contributing_dev_workshop`.


.. _contributing_doc_dependencies:

Dependencies
~~~~~~~~~~~~

The documentation build requires Python 3.11 or later.

Dependencies come from two files:

- :file:`docs/requirements.in`
  contains dependencies specific to |ws_markup| documentation.

- :file:`docs/requirements.txt`
  is the assembled file that the build installs.

The :file:`.github/workflows/update-sphinx-stack.yaml` workflow
generates :file:`docs/requirements.txt`
by appending :file:`docs/requirements.in`
to the Sphinx Stack's own requirements.
Keep constraints loose, matching the Sphinx Stack convention.
For more information about the workflow,
see :ref:`contributing_cicd`.


Choose a task
-------------

Trivial fixes such as spelling, grammar, and punctuation need no planning:
open a pull request directly.

For a larger change, such as restructuring a section or a bulk edit,
open an issue in the
`issue tracker <https://github.com/canonical/workshop/issues>`__ first
to agree on the approach.


Draft your work
---------------

Create a branch
~~~~~~~~~~~~~~~

Name your branch after the change,
following the same prefix convention as code contributions
(see :ref:`contributing_development`).
Documentation branches commonly use a :samp:`docs/` prefix,
for example :samp:`docs/contributing`.


.. _contributing_doc_structure:

Write
~~~~~

|ws_markup| uses the `Canonical documentation starter pack
<https://github.com/canonical/sphinx-stack>`_
together with a custom |ws_markup| in-project SDK in :file:`.workshop/`
to run and build its documentation;
the preferred markup is reStructuredText (reST),
with some opinionated style choices evident in the source.

See the relevant references before making changes:

- :ref:`doc_style_guide`
  (project-specific conventions and patterns)

- `Starter pack
  <https://documentation.ubuntu.com/sphinx-stack/latest/>`_

- `reST style guide
  <https://documentation.ubuntu.com/sphinx-stack/latest/reference/style-guide/>`_

- `reST cheat sheet
  <https://documentation.ubuntu.com/sphinx-stack/latest/reference/doc-cheat-sheet/>`_

The :ref:`command-line reference <ref_workshop__cli>` pages under
:file:`docs/reference/cli/`
are generated from the source, not edited by hand.
See :ref:`contributing_doc_generation` for how they're produced.


Test
~~~~

Build the documentation
and open the local preview at :samp:`127.0.0.1:8000`.
Check that your changes render as expected,
including tables, admonitions, and cross-references.

Run the remaining documentation checks,
which cover Markdown style, spelling, inclusive language, and links:

.. code-block:: console

   $ workshop run dev docs-check

For the workflows that run the same checks on your pull request,
see :ref:`contributing_cicd`.


Commit
~~~~~~

Prefix documentation commits with the :samp:`Doc:` type
and an optional scope in square brackets:

.. code-block:: none

   Doc[chore]: Align references


Review with the team
--------------------

Send for review
~~~~~~~~~~~~~~~

Submit a `pull request <https://github.com/canonical/workshop/pulls>`_,
limiting it to documentation-related files
and prefixing the title with :samp:`Doc:`.


Address quality concerns
~~~~~~~~~~~~~~~~~~~~~~~~

Your pull request runs the documentation checks:
a Sphinx build that fails on warnings, plus style, spelling, and link checks.
For the full catalog, see :ref:`contributing_cicd`.
Address any failures and push follow-up commits until the checks pass.


Wrap up the review
~~~~~~~~~~~~~~~~~~

Once the checks pass and a maintainer approves the change,
it's merged and published with the next documentation build.


Get help and support
--------------------

For documentation questions, open an issue in the
`issue tracker <https://github.com/canonical/workshop/issues>`__,
or reach the wider documentation community in the
`Ubuntu documentation Matrix channel <https://matrix.to/#/#documentation:ubuntu.com>`__.
