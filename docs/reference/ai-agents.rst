.. _ref_ai_agents:

.. meta::
   :description: Reference for Workshop's AI-agent integration points,
                 listing the LLM-readable documentation URLs, the Context7
                 integration, and the use-workshop and design-sdk
                 agentic skills.

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


.. _ref_ai_use_workshop_skill:

The use-workshop skill
----------------------

The `use-workshop-skill <https://github.com/canonical/use-workshop-skill>`_ repository
ships an agentic skill for operating the |ws_markup| CLI:
launching workshops,
refreshing them,
running commands inside,
wiring interfaces,
debugging failed changes,
and orchestrating parallel environments via Git worktrees.

If your agent supports plugins,
install the repository as a plugin:
run :samp:`/plugin marketplace add canonical/use-workshop-skill`,
then :samp:`/plugin install use-workshop@canonical`.
Otherwise,
copy :file:`.github/skills/use-workshop/` into the target repo,
using the skills path for your agent
(:file:`.claude/skills/` for Claude Code,
:file:`.github/skills/` for Copilot, and so on).
Mention |ws_markup| in any prompt to trigger the skill.


.. _ref_ai_design_sdk_skill:

The design-sdk skill
--------------------

The same repository ships :samp:`design-sdk`,
an agentic skill for the publisher side:
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
iterates with :command:`sdkcraft try` and :command:`workshop refresh`
until the SDK comes up healthy,
and writes the README.
On request,
it also onboards the SDK repository
with version branches, CI workflows, and a :file:`renovate.json`,
and publishes the SDK to the SDK Store.

The skill installs together with :samp:`use-workshop`:
the plugin carries both,
and when copying the skill directories instead,
copy :file:`.github/skills/design-sdk/`
alongside :file:`.github/skills/use-workshop/`,
as :samp:`design-sdk` reads its sibling's references.

#. Aim the agent at the new repository.

#. Run :samp:`/design-sdk new <SOFTWARE>` and answer the prompts.
   Run :samp:`/design-sdk` without arguments
   to pick one of the skill's other paths instead,
   such as :samp:`iterate`, :samp:`test`, :samp:`onboard`, or :samp:`publish`.

#. Approve the proposed design,
   then review the generated files
   and adjust where the skill's defaults don't match your case.
