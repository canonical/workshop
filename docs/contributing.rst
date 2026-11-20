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
The repository ships instructions, agents, and skills for GitHub Copilot
(see :ref:`contributing_copilot`);
to use another assistant, point it at :file:`agents.md`.

Whichever tool you use, you're the author:
be ready to explain any line of the change,
its commit messages, and its pull request description in review.
This includes pull requests that an AI agent opens at your request:
review the draft yourself before you mark it ready for review.

Commits don't need AI attribution.
For documentation drafted with AI,
run the :file:`.github/skills/documentation-review/SKILL.md` skill
before you submit.

Maintainers may close bulk or unreviewed AI-generated pull requests
without discussion.


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
