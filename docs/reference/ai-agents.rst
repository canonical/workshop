.. _ref_ai_agents:

.. meta::
   :description: Reference for Workshop's AI-agent integration points,
                 listing the LLM-readable documentation URLs, the Context7
                 integration, and the use-workshop, onboard-workshop,
                 and design-sdk agentic skills.

Workshop and AI agents
======================

.. @artefact SDK
.. @artefact workshop (container)

|ws_markup| integrates with AI coding agents,
exposing documentation as Markdown that agents can fetch and parse directly,
or retrieve through Context7,
and agentic skills that wrap |ws_markup| and |sdk_markup| operations
so agents don't have to rediscover the CLIs every session.


.. _ref_ai_discovery:

LLM-readable docs
-----------------

|ws_markup| publishes two files that follow the
`llms.txt convention <https://llmstxt.org/>`_:
`llms.txt <https://ubuntu.com/workshop/docs/llms.txt>`_
indexes every page with a one-line summary,
and `llms-full.txt <https://ubuntu.com/workshop/docs/llms-full.txt>`_
concatenates every page as Markdown.

To fetch a single page as Markdown,
append :file:`.md` to its URL.
For example,
this page is available at
:samp:`https://ubuntu.com/workshop/docs/reference/ai-agents.md`.


.. _ref_ai_context7:

Context7
--------

`Context7 <https://context7.com/canonical/workshop>`_
indexes the |ws_markup| documentation
and serves it to AI agents through its Model Context Protocol (MCP) server,
so agents can pull current docs without scraping the site.


.. _ref_ai_skills:

Agentic skills
--------------

The `use-workshop-skill <https://github.com/canonical/use-workshop-skill>`_ repository
ships three agentic skills,
one for each stage of working with |ws_markup|:

.. list-table::
   :header-rows: 1
   :widths: 3 8 6

   * - Skill
     - Use it when
     - Start with
   * - :ref:`use-workshop <ref_ai_use_workshop_skill>`
     - A workshop definition exists
       and you want the agent to operate it.
     - Mention |ws_markup| in a prompt.
   * - :ref:`onboard-workshop <ref_ai_onboard_workshop_skill>`
     - A repository has no workshop definition yet
       and you want one derived from its toolchain.
     - :samp:`/onboard-workshop onboard <REPO-PATH>`
   * - :ref:`design-sdk <ref_ai_design_sdk_skill>`
     - You publish software as an SDK
       and want the agent to design, build, and release it.
     - :samp:`/design-sdk new <SOFTWARE>`

If your agent supports plugins,
install the repository as a plugin:
run :samp:`/plugin marketplace add canonical/use-workshop-skill`,
then :samp:`/plugin install use-workshop@canonical`.
The plugin carries all three skills.

Otherwise,
copy the skill directories from :file:`.github/skills/` into the target repo,
using the skills path for your agent
(:file:`.claude/skills/` for Claude Code,
:file:`.github/skills/` for Copilot, and so on).
Always copy :file:`use-workshop/` along with the skill you need,
as the other two read its references.


.. _ref_ai_use_workshop_skill:

use-workshop
~~~~~~~~~~~~

Operates the |ws_markup| CLI on an existing definition:
launching and refreshing workshops,
running commands inside,
wiring interfaces,
debugging failed changes,
and orchestrating parallel environments via Git worktrees.
The skill triggers whenever a prompt mentions |ws_markup|
and follows every mutating command with the usual verification:
:command:`workshop changes`, :command:`workshop tasks`, :command:`workshop info`.


.. _ref_ai_onboard_workshop_skill:

onboard-workshop
~~~~~~~~~~~~~~~~

Bootstraps a definition for a repository that has none.
The skill reads how the repository already builds, tests, and runs,
delivers a feasibility verdict before writing any file,
and proposes a definition that wraps the existing entry points
(make targets, scripts, CI commands) as actions.
Once approved,
it writes :file:`.workshop/<NAME>.yaml` and any in-project SDKs,
launches the workshop,
and proves each action inside it.
The repository's own build files stay untouched.

#. Aim the agent at the repository.

#. Run :samp:`/onboard-workshop onboard <REPO-PATH>` and answer the prompts.
   Run :samp:`/onboard-workshop analyze` instead
   to stop at the feasibility verdict and proposal
   without creating anything.

#. Acknowledge the verdict and approve the proposal,
   then review the generated files.


.. _ref_ai_design_sdk_skill:

design-sdk
~~~~~~~~~~

Covers the publisher side:
designing, building, and publishing SDKs with |sdk_markup|.
The skill runs an interactive design conversation:
it asks about the software to package,
how upstream distributes it,
what must persist across refreshes,
which network services and hardware it needs,
and which bases and architectures to build for,
then proposes a design for approval.
Once approved,
it writes :file:`sdkcraft.yaml`, the hooks, and spread tests,
iterates with :command:`sdkcraft try` and :command:`workshop refresh`
until the SDK comes up healthy,
and writes the README.
On request,
it also onboards the SDK repository
with version branches, CI workflows, and a :file:`renovate.json`,
and publishes the SDK to the SDK Store.

#. Aim the agent at the new repository.

#. Run :samp:`/design-sdk new <SOFTWARE>` and answer the prompts.
   Run :samp:`/design-sdk` without arguments
   to pick one of the skill's other paths instead,
   such as :samp:`iterate`, :samp:`test`, :samp:`onboard`, or :samp:`publish`.

#. Approve the proposed design,
   then review the generated files
   and adjust where the skill's defaults don't match your case.
