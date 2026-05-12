.. _how_use_workshops_with_ai_agents:

.. meta::
   :description: How-to guide on using Workshop for parallel agentic coding,
                 demonstrating multi-agent workflows with 'claude-code' and
                 'copilot' SDKs in a shared sandbox environment.

How to use workshops with AI agents
===================================

.. @artefact workshop (container)
.. @artefact workshop definition

|ws_markup| enables multiple tools and utilities,
brought together as SDKs,
to work in sandboxed environments over a shared project repository,
thus addressing security and privacy concerns
while enabling a degree of creativity in your workflows.

Today, developer teams routinely delegate multi-step tasks to agents,
run different models for planning and implementation,
and coordinate fleets of agents across branches or repos.
In practice,
each agent and model comes with its own execution model,
sandbox mode,
and approval policies,
so |ws_markup| as a consistent container boundary is a safer baseline
than relying on every tool to self-sandbox correctly.

This guide demonstrates how to run heterogeneous AI coding SDKs
in separate Git worktrees over a shared codebase.
Using Git worktrees is a best practice recommended by
`Anthropic <https://code.claude.com/docs/en/common-workflows#run-parallel-sessions-with-worktrees>`__,
`OpenAI <https://developers.openai.com/codex/app/worktrees>`__,
and
`Cursor <https://cursor.com/blog/agent-best-practices#native-worktree-support>`__.

The guide walks through two scenarios
that build a minimal Flask app with a few HTTP routes.
Each scenario compares agent outputs
and synthesizes the optimal approach in different ways:

- Scenario 1: Parallel exploration.
  Run :program:`claude-code` and :program:`copilot` on the same task,
  then compare implementations and synthesize their best elements.

- Scenario 2: Role-based coding.
  Assign distinct roles to different agents:
  :program:`claude-code` as the architect, creating planning documents,
  :program:`copilot` as the coder, implementing the design.


Prerequisites
-------------

This guide builds on :ref:`how_git_workshops` and :ref:`how_add_actions`,
particularly the use of Git worktrees with workshops.
Worktrees help isolate and track different agents' runs and outcomes
while sharing the same project directory and the workshops in it.

Note that the :program:`claude-code` and :program:`copilot` SDKs
will prompt for login credentials on their first run;
you should have a browser window open with the respective account signed in.
Alternatively,
the agents support token-based API authentication via environment variables,
which allows you to skip the login steps below;
refer to their respective documentation and runtime help for details.

Agent prompts
-------------

Start with a fresh directory
and initialize a Git repository:

.. code-block:: console

   $ mkdir flask-project && cd flask-project
   $ git init


This is the :samp:`main` branch,
where you would normally store your codebase to be shared across worktrees.

We don't have anything yet,
so create a :file:`prompts/` directory at the repository root
to store our agent prompts instead:

.. code-block:: console

   $ mkdir prompts


Create the shared prompt for initial parallel exploration
that builds a minimal Flask app:

.. dropdown:: prompts/flask-shared.txt

   .. literalinclude:: ../../examples/prompts/flask-shared.txt


Add the synthesis prompt:

.. dropdown:: prompts/flask-synthesis.txt

   .. literalinclude:: ../../examples/prompts/flask-synthesis.txt


Create the architect prompt for Scenario 2 (relies on the shared prompt above):

.. dropdown:: prompts/flask-architect.txt

   .. literalinclude:: ../../examples/prompts/flask-architect.txt


Follow up with the coder prompt (also relies on the shared prompt above):

.. dropdown:: prompts/flask-coder.txt

   .. literalinclude:: ../../examples/prompts/flask-coder.txt


Commit the prompts
so that they are available across all worktrees
for reuse and customization:

.. code-block:: console

   $ git add prompts && git commit -m "add prompts"


Workshop definition
-------------------

.. @artefact workshop base image
.. @artefact SDK

We will be using a single workshop definition
that adds two different agents as SDKs.
You can choose a different approach for manageability.

Create a workshop definition file in the project root:

.. code-block:: yaml
   :caption: .workshop.yaml

   name: agent-dev
   base: ubuntu@24.04
   sdks:
     - name: claude-code
     - name: copilot

   actions:
     claude-auto: |
       claude --model $CLAUDE_MODEL --dangerously-skip-permissions --print "$@"

     claude: |
       claude --model $CLAUDE_MODEL --dangerously-skip-permissions "$@"

     copilot-auto: |
       copilot --model $COPILOT_MODEL --yolo --silent --prompt "$@"

     copilot: |
       copilot --model $COPILOT_MODEL --yolo --interactive "$@"


The definition adds the two SDKs,
keeping them isolated from your host system
while they work against your shared codebase.

The :samp:`actions` section defines shell commands
that encapsulate the complexity of running different agents
with their specific options and idioms;
note that all safeguards are disabled
because the workshop acts as a shared sandbox,
so there's no need to manage per-agent policies
or have these agents installed on your host.
Even with :samp:`--yolo` or :samp:`--dangerously-skip-permissions`,
any changes done by an agent remain contained inside the workshop.

Save the definition and add it to :file:`.gitignore`,
along with the :samp:`.lock` pattern:

.. code-block:: console

   $ cat >> .gitignore << EOF
   *.lock
   .workshop.yaml
   EOF


This ensures there's only one workshop definition across all worktrees;
otherwise, the definitions and lock files in different worktrees
would interfere with each other.

.. note::

   If you use multiple workshops in your project,
   add the :file:`.workshop/` directory and :samp:`*.lock` instead.

   Also, you may have valid reasons to commit these
   (team reuse, versioning);
   make sure you understand the implications.


Scenario 1: Parallel runs
-------------------------

This scenario runs two different AI agents on the same prompt,
then compares the two implementations and synthesizes an optimal approach.

Each agent runs over its own Git worktree,
which allows them to operate in parallel without interfering with each other.

However, the agents can and should share the workshop,
so launch it:

.. code-block:: console

   $ workshop launch


.. note::

   Enable autocompletion for |ws_markup|
   to speed up command entry
   and avoid mistakes in subcommands, plugs, and slots.


First worktree
~~~~~~~~~~~~~~

Create a worktree for the first agent, :samp:`claude-code`:

.. code-block:: console

   $ git worktree add claude


Next, run the first agent in non-interactive mode with the shared prompt,
specifying the model to use via an environment variable
and the worktree as the working directory:

.. code-block:: console

   $ workshop exec -- claude login  # First time only
   $ workshop run --env CLAUDE_MODEL=sonnet -w /project/claude -- \
       claude-auto "Follow the instructions in ./prompts/flask-shared.txt"


The agent generates a (presumably) complete Flask app.

In a regular development workflow,
you would iterate over the shared codebase here,
adding a feature or fixing a bug.

You don't need to wait for the run to finish;
proceed to the next step, opening a new terminal tab.


Second worktree
~~~~~~~~~~~~~~~

Create a worktree for the second agent:

.. code-block:: console

   $ git worktree add copilot


Run the second agent in non-interactive mode with the same prompt,
specifying the model to use via an environment variable
and the new worktree as the working directory:

.. code-block:: console

   $ workshop exec -- copilot  # First time only: login
   $ workshop run --env COPILOT_MODEL=gpt-5.1-codex -w /project/copilot -- \
       copilot-auto "Follow the instructions in ./prompts/flask-shared.txt"


In a regular development workflow,
you would expect this to produce an alternative solution to the same problem.


Synthesis
~~~~~~~~~

Create a third worktree
where a third run will compare and join both implementations:

.. code-block:: console

   $ git worktree add synthesis


Run the :samp:`claude-code` agent in interactive mode
with a smarter model and the architect prompt,
supplying the worktree as the working directory:

.. code-block:: console

   $ workshop run --env CLAUDE_MODEL=opus -w /project/synthesis -- \
       claude "Follow the instructions in ./prompts/flask-synthesis.txt"


The synthesis agent walks through both implementations interactively,
asking questions:

.. code-block:: text

   Q: Implementation A keeps everything in :file:`app.py`,
      while Implementation B splits out helpers.
      Which layout do you prefer?

   A: option B


After gathering your preferences,
the agent creates the final synthesized implementation.

In a regular development workflow,
you would cherry-pick the best design choices between the two alternatives,
eventually merging the result into :samp:`main`.


Scenario 2: Role-based coding
-----------------------------

This scenario demonstrates a different workflow
where agents take on distinct roles:
one as an architect creating planning documents,
another as a coder implementing the design.


Design worktree
~~~~~~~~~~~~~~~

Start fresh from the project root,
creating the design worktree:

.. code-block:: console

   $ git worktree add design


Run the :samp:`claude-code` agent in interactive mode
with a smarter model and the synthesis prompt,
supplying the worktree as the working directory:

.. code-block:: console

   $ workshop run --env CLAUDE_MODEL=opus -w /project/design -- \
       claude "Follow the instructions in ./prompts/flask-architect.txt"


The agent eventually creates several planning documents in the worktree.

In a regular development workflow,
you would iterate over the design,
testing some rapid prototypes to validate the approach
and refining the plans until they are ready.


Implementation worktree
~~~~~~~~~~~~~~~~~~~~~~~

Next, create the implementation worktree:

.. code-block:: console

   $ git worktree add implementation


Run the implementation agent in non-interactive mode with the coder prompt,
supplying the worktree as the working directory:

.. code-block:: console

   $ workshop run --env COPILOT_MODEL=gemini-3-pro-preview -w /project/implementation -- \
       copilot-auto "Follow the instructions in ./prompts/flask-coder.txt"


The coder agent reads the planning documents,
tracked in a separate branch,
and (presumably) implements the design in one go.

In a regular development workflow,
you would use the coder to implement new features and fix bugs,
often switching between the design and implementation worktrees.


Conclusion
----------

The scenarios above demonstrate |ws_markup| usage with one example.
For your own projects,
adapt the scenarios to your orchestration needs.
Treat them as a starting point,
not a template that must fit every use case.

|ws_markup| provides a versatile environment
for development workflows that involve multiple complex tools such as AI agents.
The scenarios above demonstrate two popular patterns,
but real-world use cases could also include:

- Hybrid approach with a single architect and multiple parallel coders

- Evals and benchmarks for side-by-side comparison

- Multi-layered role orchestration across many branches

- Additional personas, such as analyst, tester, or product owner,
  each with their own branch and agentic stack;
  extra capabilities such as skills or subagents can be used, too


By running agents in isolated worktrees
with a shared workshop sandbox,
you gain the benefits of security and privacy
while having the flexibility
to mix and match different toolchains and workflows.


See also
--------

Explanation:

- :ref:`exp_workshop_definition`
- :ref:`exp_workshop_definition_actions`


Reference:

- :ref:`ref_workshop_run`
- :ref:`ref_workshop_exec`
- :ref:`ref_workshop_actions`
