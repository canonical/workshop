.. _ref_ai_agents:

.. meta::
   :description: Reference for Workshop's AI-agent integration points,
                 listing the LLM-readable documentation URLs, the Context7
                 integration, the use-workshop, onboard-workshop,
                 and design-sdk agentic skills, and how to find
                 and assess agent SDKs.

Workshop and AI agents
======================

.. @artefact SDK
.. @artefact workshop (container)

|ws_markup| integrates with AI coding agents,
exposing documentation as Markdown that agents can fetch and parse directly,
or retrieve through Context7,
agentic skills that wrap |ws_markup| and |sdk_markup| operations
so agents don't have to rediscover the CLIs every session,
and SDKs that install the agents themselves in a workshop.


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


.. _ref_ai_agent_sdks:

Agent SDKs
----------

.. @artefact sdk find
.. @artefact sdk info

Coding agents are published as SDKs on the SDK Store.
Search for them with :command:`sdk find`:

.. code-block:: console

   $ sdk find agent

     NAME                  VERSION     PUBLISHER            SUMMARY
     ...
     claude-code           2.1.273     Canonical✓           Claude Code CLI
     codex                 0.156.1     Canonical✓           OpenAI Codex CLI agent
     ...
     copilot               1.0.88      Canonical✓           GitHub Copilot CLI - AI-powered coding assistant for the terminal
     ...


The query matches an SDK's name, title, summary, description, or publisher,
so the results also include tools for agents,
such as memory servers and skill managers,
and SDKs that only mention agents in their descriptions.

The mark after a publisher's name
shows the publisher's validation status in the SDK Store.
When the output doesn't go to a terminal with a UTF-8 locale,
for example when it's piped to another command,
:command:`sdk` prints an ASCII fallback instead:

.. list-table::
   :header-rows: 1
   :widths: 2 2 6

   * - Mark
     - Fallback
     - Publisher
   * - :samp:`✓`
     - :samp:`**`
     - Verified, such as Canonical
   * - :samp:`✪`
     - :samp:`*`
     - Starred
   * - None
     - None
     - Not validated

Before adding an agent SDK to a workshop,
inspect it with :command:`sdk info`:

.. code-block:: console

   $ sdk info claude-code

     name:       claude-code
     publisher:  Canonical✓
     license:    https://www.anthropic.com/legal/commercial-terms
     website:    https://github.com/canonical/claude-code-sdk

     ...

     CHANNELS
       CHANNEL           VERSION  BUILD       BASE  REV     SIZE
       latest/stable     2.1.273  2026-09-24  all    37  88.10MB
       latest/candidate  ↑
       latest/beta       ↑
       latest/edge       ↑


Check these fields before you rely on an agent SDK:

.. list-table::
   :header-rows: 1
   :widths: 2 8

   * - Field
     - What to check
   * - :samp:`publisher`
     - Who publishes the SDK, with the same validation mark as in :command:`sdk find`.
       The account name follows in parentheses
       when it differs from the display name.
   * - :samp:`license`
     - The terms that cover the agent the SDK installs;
       for a proprietary agent, this is often a link to the vendor's terms.
   * - :samp:`website`
     - Where the SDK's source lives,
       so you can review its hooks, plugs, and README before you install it.
   * - :samp:`CHANNELS`
     - The tracks and risk levels that the SDK is published on,
       with the version, base, and revision each channel offers.
       A :samp:`↑` means that the risk level has no revision of its own
       and follows the one above it.
       Pass :option:`!--arch` :samp:`all` to list every architecture.

An agent SDK installs the agent in the workshop,
where it runs as the :samp:`workshop` user like every other command.
Before you let an agent work without its approval prompts,
review what the workshop does and doesn't protect
in the :ref:`security policy <security_coding_agents>`.


See also
--------

Explanation:

- :ref:`exp_multi_workshop_patterns`
- :ref:`security_coding_agents`


Reference:

- :ref:`ref_sdk_find`
- :ref:`ref_sdk_info`
