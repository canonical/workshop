.. _contributing:

.. meta::
   :description: Contribute to the Workshop project. View the contribution
                 process, standards, and the ways to get involved as a
                 developer or a documentation author.

Contribute
==========

We believe everyone has something valuable to contribute,
whether you're a coder, a writer, or a tester.
Here's how and why you could get involved:

- **Why join us**:
  Work with like-minded people, grow your skills,
  connect with diverse professionals, and make a difference.

- **What do you get**:
  Personal growth, recognition for your contributions,
  early access to new features, and the joy of seeing your work appreciated.

- **Start early, start simple**:
  Dive into code contributions,
  improve documentation, or be among the first testers.
  Your presence matters, regardless of experience or the scale of your input.


Standards and expectations
--------------------------

|ws_markup| is an emergent project
stewarded by the DevEx department at Canonical.
Before you start, familiarize yourself with two documents:

- The `Ubuntu Code of Conduct <https://ubuntu.com/community/docs/ethos/code-of-conduct>`__
  sets the standard for respectful collaboration across the community.

- The `license <https://github.com/canonical/workshop/blob/main/LICENSE>`__
  governs how the project's code is used and distributed.


.. _contributing_ai:

Acceptable use of AI
~~~~~~~~~~~~~~~~~~~~

AI assistance is welcome in contributions of any kind.
The repository is tuned for GitHub Copilot:
it includes instructions and agents tailored to the project,
along with documentation skills sourced from Canonical's shared
`copilot-collections <https://github.com/canonical/copilot-collections>`__ set
(see :ref:`contributing_copilot`).
Try Copilot first;
other assistants are just as acceptable.

Whichever tool you use, authorship stays with you:
submit AI-assisted work as your own
and take full responsibility for its contents.

In practice:

- Copilot automatically reviews each pull request against :samp:`main`,
  except drafts, and reviews it again after every push.
  Treat its comments like any other reviewer's.

- When you commit a Copilot suggestion from the pull request page,
  keep the ``Co-authored-by`` trailer that GitHub adds,
  but reword the title to follow the commit conventions for
  :ref:`code <contributing_dev_commit>`
  or :ref:`documentation <contributing_doc_commit>`.
  Other AI attribution isn't needed.

- Write commit messages and code comments yourself:
  they're the project's long-term memory.

- Use AI freely to draft pull request descriptions;
  check that they match the change,
  and point out AI-generated code that reviewers should know about.

- Draft documentation with AI if it helps,
  then edit it to comply with the :ref:`doc_style_guide`
  and run the documentation review skill before you submit.

Bulk or low-effort AI-generated pull requests
that disregard the project's practices
are closed at the maintainers' sole discretion,
without extended discussion.


Ways to contribute
------------------

Pick the path that matches the work you want to do:

- :ref:`contributing_development` takes a code change from a local
  environment through to a merged pull request.

- :ref:`contributing_documentation` follows the same arc for changes to
  this documentation.

- Report a bug or share feedback through the
  `issue tracker <https://github.com/canonical/workshop/issues>`__.

Maintainers can find the release process, automation, and repository tooling
in :ref:`contributing_maintenance`.


Feedback and issues
-------------------

Before opening a new issue, search the
`existing issues <https://github.com/canonical/workshop/issues>`__
to avoid duplicates and to see whether the topic is already being worked on.


.. toctree::
   :hidden:

   Development <contributing/development>
   Documentation <contributing/documentation>
   Maintenance <contributing/maintenance>
