```{eval-rst}
:orphan:

.. meta::
   :description: Workshop documentation style guide covering file naming,
                 structure, semantic line breaks, reStructuredText and Markdown
                 conventions, terminology, and project-specific patterns.
```

(doc_style_guide)=

# Workshop documentation style guide

This style guide documents the established conventions used in the Workshop documentation. It captures actual patterns observed across the documentation set and serves as a reference for maintaining consistency in new contributions.

This guide is subordinate to Canonical's documentation standards but records Workshop-specific decisions and patterns that extend or clarify those standards.

---

## File naming and organization

**Directory structure**

The documentation follows the [Diátaxis](https://diataxis.fr/) framework with four main sections:

```
docs/
├── tutorial/          # Step-by-step learning paths
├── how-to/            # Task-oriented guides
├── explanation/       # Conceptual information
└── reference/         # Technical specifications
```

**File naming convention**

All filenames use lowercase letters and dashes for word separation.

Examples:

- Good: `part-1-get-started.rst`
- Good: `connect-vscode.rst`
- Good: `camera-interface.rst`
- Good: `sdk-vs-dockerfile.rst`
- Avoid: `ConnectVSCode.rst` (uppercase)
- Avoid: `camera_interface.rst` (underscore)

Tutorial files use a sequential numbering pattern:

```
part-1-get-started.rst
part-2-work-with-interfaces.rst
part-3-sketch-sdks.rst
part-4-craft-sdks.rst
```

How-to files: Use verb-first naming pattern:

```
add-actions.rst
connect-vscode.rst
forward-ports.rst
debug-issues.rst
resolve-plug-conflicts.rst
```

Explanation files use noun-based naming:

```
concepts.rst
camera-interface.rst
best-practices.rst
runtime-behavior.rst
```

Reference files match command structure:

```
workshop-launch.rst
workshop-connect.rst
sdkcraft-build.md
```

Filenames and directory names in the documentation repo should be in lowercase,
with dashes instead of spaces; the directory tree must be built in a way that
provides for readable, meaningful URLs: `/docs/howto/change-tyres`.

---

## Page structure and metadata

**Standard page structure**

Every reStructuredText documentation page follows this structure:

```restructuredtext
.. _anchor_label:

.. meta::
   :description: Brief description for search engines and social media

Page Title
==========

Opening paragraph providing context and purpose.

Section Heading
---------------

Content...

Subsection Heading
~~~~~~~~~~~~~~~~~~

Content...
```

**Metadata block**

Every page must have a `.. meta::` block immediately after the anchor label.

Format:

```restructuredtext
.. meta::
   :description: A brief, clear description of the page content for SEO and
                 social media. Typically 1-2 lines, wrapping at natural phrase
                 boundaries.
```

Examples from the documentation:

```restructuredtext
.. meta::
   :description: Practical introduction to workshops, guiding users through
                 defining, launching, and refreshing workshops, and executing commands in workshops.
```

```restructuredtext
.. meta::
   :description: A comprehensive explanation of the Workshop interface system,
                 detailing how SDKs connect to host system resources through
                 interfaces, and the mechanism of plugs and slots for resource
                 sharing between containers.
```

**Anchor labels**

Use lowercase with underscores, prefixed by section type.

Prefixes:

- `tut_` - Tutorial sections
- `how_` - How-to guides
- `exp_` - Explanation articles
- `ref_` - Reference documentation

Examples:

```restructuredtext
.. _tut_get_started:
.. _how_add_actions:
.. _exp_interface_concepts:
.. _ref_workshop_launch:
```

**Artefact comments**

Use `.. @artefact` comments to mark key concepts for coverage tracking:

```restructuredtext
.. @artefact workshop (container)
.. @artefact SDK
.. @artefact interface
.. @artefact workshop launch
```

The current list of these concepts is maintained in `docs/coverage.yaml`;
update it as needed.

**Prerequisites sections**

State a page's prerequisites in a dedicated `Prerequisites` section so readers
can confirm they are ready before following any steps.

- In a how-to, place `Prerequisites` as a top-level section (H2, dashes)
  immediately after the opening paragraph and before the first procedure section.
  In a tutorial that installs Workshop, place it as a subsection (H3, tildes)
  under the `Install` section instead.
- Open the section with `Before starting, ensure you have these requirements satisfied:`,
  followed by a bullet list with one requirement per item.
- List only genuine, non-obvious prerequisites: specific external tools, accounts,
  or hardware, and version-pinned dependencies such as LXD 6.8 or later.
- Omit the universally assumed baseline (an existing `workshop.yaml`, the Workshop
  CLI, an Ubuntu or snap-enabled host) unless a particular version or
  configuration matters. If a page needs nothing beyond that baseline, omit the
  section rather than pad it.

```restructuredtext
Prerequisites
-------------

Before starting, ensure you have these requirements satisfied:

- LXD 6.8 or later running on the host.

- An Ubuntu One account.
```

---

## Writing style and tone

**Voice and audience**

Target audience is developers and DevOps professionals seeking to:

* Achieve specific goals without much overhead and roundabout musings
* Perform and conceive complex ad-hoc tasks and workflows that require precision and depth
* Attain understanding of Workshop's key capabilities beneficial for their scenarios

Content follows the Diátaxis framework, providing:

* Concise tutorials for common, starter-level actions and scenarios, eliminating the need to invent custom steps and allowing novice users to journey along the hot path effortlessly
* Elaborate explanations of the thinking behind Workshop's design, including design decisions, related concepts, and how it should be used
* Detailed how-to guides that address specific needs of advanced users and cover topics beyond basic entry-level operations
* Comprehensive reference of all options, settings, and details available to customize Workshop's operation in any desirable manner

The tone is authoritative but relaxed, confident but approachable. Think water cooler conversation, not classroom session.

Example from the documentation:

```text
Workshop is a tool for defining and handling ephemeral development environments.

List your dependencies and components in YAML to define an environment. The key pieces of a definition are SDKs, independent but connectable units of functionality created by software publishers and available on the SDK Store. Workshop simplifies experiments with your environment layout.
```

**Direct instructions**

Use imperative mood for instructions. Avoid "you can" or "you may" for required actions.

Preferred:

```
Install Workshop using the --classic option:
```

Avoid:

```
You can install Workshop with:
```

**Paragraph length**

Keep paragraphs focused and relatively short (2-5 sentences typically). Complex topics should be broken into multiple paragraphs.

Example from tutorial:

```restructuredtext
Install Workshop,
upgrading the prerequisites if needed,
then ensure it runs.

Authenticate to the Snap Store and install the snap
using the `--classic <...>`_ option:
```

**Clarity over cleverness**

- State prerequisites explicitly
- Define terms at first use
- Avoid assumptions about reader knowledge
- Use precise, unambiguous language

**Language and spelling**

Convention: Use US English spelling, grammar, and formatting conventions throughout the documentation.

Examples:
- Good: `color`, `center`, `analyze`, `behavior`
- Avoid: `colour`, `centre`, `analyse`, `behaviour`
- Good: Use serial comma: "SDKs, interfaces, and workshops"
- Good: Double quotes for quotations: "Workshop is a tool"

---

## Semantic line breaks

**Pattern**

The documentation consistently uses semantic line breaks (one line per clause or significant phrase) in reStructuredText files. This improves version control diffs and editing precision.

Rationale: Semantic breaks make git diffs more readable and help reviewers identify exactly what changed in a sentence or paragraph.

**Implementation**

Break lines at natural semantic boundaries:
- After each complete clause
- Before coordinating conjunctions (and, but, or)
- Before relative clauses (which, that, who)
- After introductory phrases

Example from the documentation:

```restructuredtext
This is the first section of the :ref:`four-part series <tut_index>`;
a practical introduction
that takes you on a tour
of the essential |ws_markup| activities.
```

```restructuredtext
To make use of these interfaces,
SDKs and :ref:`workshops <exp_workshop_definition_connections>` define *slots*.
For example, a :ref:`mount interface <exp_mount_interface>` slot
creates a source directory to be mounted inside the workshop via a plug.
```

```restructuredtext
When crafting SDKs for |ws_markup|,
publishers face design decisions
that affect how their SDKs install, integrate, and work inside workshops.
Understanding the best practices outlined below
helps publishers create more maintainable, reliable, and user-friendly SDKs
that better align with |ws_markup|'s architecture and ideology.
```

**When to break**

Break after:
- Complete independent clauses
- Introductory prepositional phrases
- Transitional phrases
- Items in a complex series

Keep together:
- Short phrases that form a single unit
- Inline markup and its target word
- Cross-reference markup

Example:

```restructuredtext
Interfaces are a mechanism for communication and resource sharing.
It is an integral part of workshop confinement,
ensuring that each workshop operates in its own isolated environment,
while still allowing controlled interactions among the SDKs and with the host.
```

---

## Headings and titles

**Capitalization**

Pattern: Sentence case for all headings (capitalize only first word and proper nouns).

Examples:

```restructuredtext
Get started with workshops
==========================

Install |ws_markup|
-------------------

Prerequisites
~~~~~~~~~~~~~
```

Exception: Product names and proper nouns maintain their capitalization:

```restructuredtext
How to use JetBrains Gateway with Workshop
==========================================
```

**Heading hierarchy**

reStructuredText heading levels (consistent across documentation):

```restructuredtext
Page Title (H1)
===============

Section (H2)
------------

Subsection (H3)
~~~~~~~~~~~~~~~

Sub-subsection (H4)
^^^^^^^^^^^^^^^^^^^
```

### How-to title pattern

How-to guides follow the pattern: "How to [action] [object]"

Examples:

```
^^^^^^^^^^^^^^^^^^^^
```

**How-to title pattern**

How-to guides follow the pattern: "How to [action] [object]":
- How to forward ports with tunneling
- How to fix plug conflicts with binding
- How to debug issues in workshops

Linking exception: In navigation and links, drop "How to" prefix and use infinitive:

```restructuredtext
How-to guides:

* Debug issues in workshops
* Connect VS Code to a workshop
```

---

## reStructuredText conventions

**Code blocks**

Standard format:

```restructuredtext
.. code-block:: console

   $ workshop launch dev
```

```restructuredtext
.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@22.04
```

With emphasis:

```restructuredtext
.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 7-9

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: 1.26
   
   actions:
     lint: |
       golangci-lint run
```

Supported languages: `console`, `yaml`, `python`, `go`, `shell`, `ini`, `json`

**Admonitions**

Note:

```restructuredtext
.. note::

   For other ways to install LXD,
   see the available installation options in
   `LXD documentation <...>`_.
```

Warning:

```restructuredtext
.. warning::

   This will permanently delete all workshop data.
```

**Placement:** Place admonitions at the end of the subsection they relate to, rather than interrupting the flow of text in the middle of a section.

**Inline markup**

Semantic markup preference: Use semantic markup roles (`:samp:`, `:envvar:`, `:file:`, etc.) instead of generic ones (\`, \*, etc.). Choose the most specific role that suits the purpose and use it consistently.

Emphasis (italics):

```restructuredtext
A *workshop* is a development environment running in a container.
```

Use italics sparingly to introduce new terms (a link is even better) and for emphasis. Leave bold for product names and commands.

Strong (bold): Rarely used; prefer other markup when possible.

Program/command names:

```restructuredtext
:program:`workshop`
:command:`workshop launch`
```

Commands in `:command:` roles should be presented in their complete form (e.g. `workshop launch`, not just `launch`) and should not be used as verbs or nouns in the text. Use non-breaking spaces in inline command literals to prevent longer compound commands from wrapping.

File paths:

```restructuredtext
:file:`workshop.yaml`
:file:`/home/user/.ollama/models/`
```

End directory path names with a slash where possible and conventional to disambiguate directories from files.

Sample values:

```restructuredtext
:samp:`ollama`
:samp:`ssh-agent`
```

Environment variables:

```restructuredtext
:envvar:`PATH`
:envvar:`HOME`
```

Placeholders:
Format placeholders in uppercase within angle brackets, without underscores:

```restructuredtext
:samp:`workshop launch {WORKSHOP}`
:samp:`{SDK-NAME}@{CHANNEL}`
```

Or in documentation text:

```
workshop launch <WORKSHOP>
```

Substitutions are reusable text replacements defined in `docs/reuse/substitutions.txt` and automatically included in all reStructuredText files:

```restructuredtext
|ws_markup|    # Renders as :program:`Workshop`
|sdk_markup|   # Renders as :program:`SDKcraft`
```

These ensure consistent formatting of product names throughout the documentation. Use them instead of typing product names manually.

Common external links are defined in `docs/reuse/links.txt` for consistent reference across documentation:

```restructuredtext
.. _Canonical website: https://canonical.com/
.. _GitHub: https://github.com/canonical/workshop/
.. _LXD: https://canonical.com/lxd/docs/latest/
.. _SDKcraft: https://github.com/canonical/sdkcraft/
.. _Releases: https://github.com/canonical/workshop/releases/
```

Reference these with trailing underscores:

```restructuredtext
See the `GitHub`_ repository for source code.
Refer to the `LXD`_ documentation for setup details.
```

**Non-breaking spaces:** Use non-breaking spaces (U+00A0 or `~` in LaTeX contexts) for important proper names and inline compound commands where line breaks would be awkward, though this is rarely needed in reStructuredText.

**Lists**

Bulleted lists:

```restructuredtext
- Camera interface (manually connected)
- Desktop interface (manually connected)
- GPU interface (auto-connected)
```

Numbered lists: Use pound signs for auto-numbering:

```restructuredtext
#. First step
#. Second step
#. Third step
```

Multiline list items: Separate items with a blank line for visibility if at least one item is multiline:

```restructuredtext
- First item with a longer description
  that spans multiple lines

- Second item that is also long
  and needs proper spacing

- Third item
```

**Table of contents**

Follow this pattern, avoiding hidden ToCs where possible:

```restructuredtext
Heading
=======

Some summary of what's to follow.

These articles say this and this:

.. toctree::
   :glob:
   :maxdepth: 1

   *

These articles say this and this:

.. toctree::
   :glob:
   :maxdepth: 1

   *
```

**"See also" sections**

"See also" sections can appear on pages under any pillar and link to related content not immediately essential but potentially useful. Break link lists down by pillar, listing pillars and individual subsections in alphabetical order:

```restructuredtext
See also
--------

Explanation:

* :ref:`exp_sdks`
* :ref:`exp_workshop`

How-to guides:

* :ref:`how_resolve_plug_conflicts`

Reference:

* :ref:`ref_cli`
* :ref:`ref_workshop_connect`
* :ref:`ref_workshop_info`
```

Or using custom link text:

```restructuredtext
See also
--------

Explanation:

* :ref:`changes, tasks (concepts) <exp_changes_tasks>`
* :ref:`project (concept) <exp_project>`
* :ref:`workshop (concept) <exp_workshop>`, :ref:`workshop definition (file) <exp_workshop_definition>`

Reference:

* :ref:`workshop changes (command) <ref_workshop_changes>`
```

Links listed here must point to concepts used in the main document;
vice versa, concepts appearing in the main text should be linked in "See also"
if a well-established related page exists.

Special case: If "See also" is the only subsection on the page, hide the sidebar ToC on the right using the `:hide-toc:` directive at the top of the file.

**Tab headings**

Pattern: Keep tab headings noun-based and consistent across related content. Avoid "sticky toggling" (where tab state persists inappropriately across different contexts).

Example:

```restructuredtext
.. tabs::

   .. tab:: Ubuntu

      Installation instructions for Ubuntu...

   .. tab:: macOS

      Installation instructions for macOS...
```

**Rubric directive**

Used in CLI reference for section headers:

```restructuredtext
.. rubric:: Usage

.. code-block:: console

   $ workshop launch <WORKSHOP>... [flags]

.. rubric:: Description

This command constructs the workshops...

.. rubric:: Examples

Launch the 'nimble' and 'jazzy' workshops:
```

**Sphinx extensions and roles**

Preference: Use Sphinx-specific [roles](https://www.sphinx-doc.org/en/master/usage/restructuredtext/roles.html) and [directives](https://www.sphinx-doc.org/en/master/usage/restructuredtext/directives.html) over `docutils` generic equivalents. Use all their options and capabilities, listing options in alphabetical order.

Example with options:

```restructuredtext
.. code-block:: yaml
   :caption: workshop.yaml
   :emphasize-lines: 3-5
   :linenos:

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: 1.26
```

**Spacing and formatting**

Use three spaces to indent reStructuredText directives and separate directive content with a blank line:

```restructuredtext
.. tab::

   .. code-block:: console
   
      $ workshop launch dev
```

Separate `list-table` rows with blank lines:

```restructuredtext
.. list-table::

   * - **Tutorial**
     - :ref:`Get started <tut_get_started>` • ...

   * - **Workshops**
     - :ref:`Concepts <exp_workshop_concepts>` • ...
```

Section gaps: Include a noncumulative two-line gap (two blank lines) after code samples, lists, tables, and before headings for visual clarity.

Examples from the documentation:

After code blocks:

```restructuredtext
.. code-block:: console

   $ sudo snap install --classic workshop


Prerequisites
~~~~~~~~~~~~~
```

After lists:

```restructuredtext
- :command:`workshop stop` doesn't destroy the workshop,
  unlike :ref:`remove <tut_remove>`

- :command:`workshop start` doesn't build it from scratch,
  unlike :ref:`launch <tut_launch>` or :ref:`refresh <tut_refresh>`


In the next step, you'll refresh an existing workshop.
```

After tables:

```restructuredtext
.. list-table::
  :header-rows: 1
  :widths: 25 75

  * - Component Type
    - Description

  * - Runtime components
    - Core binaries and libraries that change infrequently


However, parts are not mandatory:
```

Before headings:

```restructuredtext
The actions you're about to perform
cover most of your daily needs with |ws_markup|.


.. _tut_install:

Install |ws_markup|
-------------------
```

---

## Markdown style conventions

This section covers Markdown-specific conventions used in the Workshop documentation. For general writing style, see the "Writing style and tone" section above.

### When to use Markdown

Markdown is used for:
- Release notes (`release-notes/v0.*.md`)
- Auto-generated CLI reference (`reference/cli/sdkcraft/*.md`)
- Special files rendered on GitHub (`security.md`, `coverage.md`)

For all other documentation, prefer reStructuredText (`.rst` files).

### File naming

Use the version number as the filename for release notes: `vX.Y.Z.md`.

### Metadata blocks

Pattern: Markdown files should include metadata using the `{eval-rst}` directive at the top of the file.

Required for:
- Release notes (`release-notes/v0.*.md`)
- Any Markdown documentation files that will be rendered in Sphinx

Format:

````markdown
```{eval-rst}
.. meta::
   :description: Brief description for search engines and social media.
```

# Page Title
````

Exception: Auto-generated CLI reference files for SDKcraft (`reference/cli/sdkcraft/*.md`) do not require metadata blocks, as they are automatically generated from command definitions.

Example from release notes:

````markdown
```{eval-rst}
.. meta::
   :description: Release notes for Workshop v0.1.28, highlighting key changes,
                 new features, and bug fixes in this version.
```

# Workshop v0.1.28 release notes
````

### Headings

Use ATX-style headers (`#`) with sentence case (capitalize only first word and proper nouns):

```markdown
# Page title

## Section heading

### Subsection heading
```

### Links

Use inline link syntax:

```markdown
See [Privately reporting a security vulnerability](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability) for instructions.
```

Avoid reference-style links in Markdown files.

### Code blocks

Use fenced code blocks with language specifiers:

````markdown
```console
workshop launch dev
```

```yaml
name: dev
base: ubuntu@22.04
```
````

### Lists

Use hyphens (`-`) for unordered lists:

```markdown
- First item
- Second item
- Third item
```

Use numbers for ordered lists:

```markdown
1. First step
2. Second step
3. Third step
```

### Emphasis

- *Italics*: Use single asterisks `*text*`
- **Bold**: Use double asterisks `**text**`

### Release notes template

Use the following template for new release notes, ensuring all links and version numbers are updated:

````markdown
```{eval-rst}
.. meta::
   :description: Release notes for Workshop X.Y.Z and SDKcraft A.B.C,
                 highlighting [key features].
```

# Workshop X.Y.Z release notes

## [Day] [Month] [Year]

These release notes cover new features and changes in Workshop X.Y.Z and
SDKcraft A.B.C.

## Requirements and compatibility

Workshop relies on Snap and LXD:

- See the [Tutorial](https://ubuntu.com/workshop/docs/tutorial/) for setup instructions.
- Refer to the [Contribution Guide](https://ubuntu.com/workshop/docs/contributing/) for development prerequisites.

## What's new in X.Y.Z

[Brief summary of the release].

### Workshop

**Repository:** <https://github.com/canonical/workshop>

- [Change and its benefit].

### SDKcraft A.B.C

**Repository:** <https://github.com/canonical/sdkcraft>

- [Change and its benefit].

----

**Full Changelog**:

- Workshop: https://github.com/canonical/workshop/compare/vX.Y.Z-1...vX.Y.Z
- SDKcraft: https://github.com/canonical/sdkcraft/compare/A.B.C-1...A.B.C
````

Add one `###` section per repository that changed in the release, each with its
`**Repository:**` link. Besides Workshop and SDKcraft, past releases have used
Reference SDKs, Docs, Workshop GitHub Action, and VS Code Workshop extension.

Workshop and SDKcraft are versioned independently, so the two numbers rarely
match. Write versions bare in prose and headings; the `v` prefix belongs only in
filenames and in Workshop tag and compare URLs, since SDKcraft tags carry no
prefix.

When SDKcraft did not release alongside Workshop, carry its last released version
forward and mark the heading:

```markdown
### SDKcraft A.B.C (unchanged)

**Repository:** <https://github.com/canonical/sdkcraft>

- No changes in this release.
```

In that case, omit the SDKcraft line from the changelog footer: there is no diff
to link. Either way, record the pairing in the version table in
`docs/release-notes/index.rst`.

### Simplified markup for GitHub-rendered files

Use simplified markup for files that have special meaning on GitHub and need to be rendered there (such as `docs/readme.rst` or `docs/contributring.rst`). These files should be more accessible to users viewing them directly.

Key simplifications:

- No `$` prompts in code blocks: GitHub doesn't prevent prompt selection during copying, which can confuse users.
- No semantic roles: Fall back to generic markup instead (e.g., use double backticks for code instead of `:command:` or `:program:`).
- Plain link syntax: Use simple inline links instead of reference-style links.

Examples:

```restructuredtext
.. code-block:: console

   sudo snap install --classic workshop
```

Notice: No `$` prompt, making it easier to copy commands directly.

Notice: Plain inline links instead of reference-style links, generic double backticks instead of semantic roles.

---

## Code examples

**Console examples**

Pattern: Show command with prompt, followed by output (if relevant):

```restructuredtext
.. code-block:: console

   $ workshop launch dev

     Launching dev...
     Launched dev
```

Command prompts: Use the nonselectable `$` prompt. The `console` lexer in `.. code-block::` automatically handles this, making the prompt nonselectable during copy operations.

Root access: When root access is required, include `sudo` explicitly:

```restructuredtext
.. code-block:: console

   $ sudo snap install workshop --classic
```

Command output: Indent output with two spaces and separate it from the command with a blank line:

```restructuredtext
.. code-block:: console

   $ workshop list --global

   PROJECT          WORKSHOP  STATUS   NOTES
   ~/myproject      dev       Running  -
   ~/otherproject   test      Stopped  -
```

Comments in commands: Use two forms for comments:

```restructuredtext
.. code-block:: console

   # Full line comment explaining the command
   $ workshop launch dev

   $ workshop exec dev -- echo "test"  # Inline comment with two spaces before #
```

**Configuration examples**

Always include caption when it can be deduced from context:

```restructuredtext
.. code-block:: yaml
   :caption: workshop.yaml

   name: dev
   base: ubuntu@22.04
   sdks:
     - name: go
       channel: 1.26
```

Indentation: Use commonly recognized formatting:
- YAML files: 2-space indentation
- JSON files: 4-space indentation

**Multi-line shell commands**

Use backslash continuation or explicit line breaks:

```restructuredtext
.. code-block:: console

   $ workshop connect dev/ollama:host 127.0.0.1:11434 \
       --host-port 11434
```

---

## Cross-references and links

**Internal cross-references**

Preferred method: Use `:ref:` links with semantic labels, not paths:

```restructuredtext
:ref:`tut_get_started`
:ref:`how_add_actions`
:ref:`exp_interface_concepts`
```

With custom text:

```restructuredtext
:ref:`four-part series <tut_index>`
:ref:`workshop definition <exp_workshop_definition_connections>`
```

Avoid `:doc:` links: Use `:doc:` links sparingly and only in specific contexts where finer manual control over table of contents lists is needed. Currently acceptable uses:

- Home page (`index.rst`) for primary navigation structure
- Release notes (`release-notes/index.rst`) for version listings

For all other internal documentation links, prefer `:ref:` with semantic anchor labels, as they are more robust to file reorganization and provide better error checking.

**External links**

Inline:

```restructuredtext
`LXD documentation <https://canonical.com/lxd/docs/latest/>`_
```

Anonymous:

```restructuredtext
See the `Snapcraft guide <https://snapcraft.io/docs/>`__ for details.
```

**Link text guidelines**

Avoid: Generic "click here" or "see this" text

Prefer: Descriptive phrases integrated into the sentence

Example:

Good:

```
See the available installation options in LXD documentation.
```

Avoid:

```
See here for more details.
```

**First mention pattern**

Link important terms only at first mention on a page. Avoid excessive linking.

**Reference label convention**

Use the following underline convention for `:ref:` anchor labels:

```restructuredtext
.. _ref_workshop_launch:
.. _how_add_actions:
.. _exp_interface_concepts:
.. _tut_get_started:
```

Pattern: `.. _{prefix}_{descriptive_name}:` where prefix indicates the section type (ref/how/exp/tut).

---

## Terminology, product names

**Product names**

Workshop - Always capitalized, never "workshop" when referring to the product.

SDKcraft - Always use capital SDK, never "Sdkcraft" or "sdkcraft".

LXD - Always uppercase.

**Technical terms**

workshop (lowercase) - The container environment itself:

```
A workshop is a development environment running in a container.
```

SDK - Always uppercase. Plural: SDKs (no apostrophe).

interface - Lowercase when referring to the general concept; specific interfaces follow same pattern:
- camera interface
- GPU interface  
- mount interface

**Command names**

Always use exact command syntax:

```
workshop launch
workshop connect
workshopctl
sdkcraft build
```

**Substitutions and reusable content**

**Text substitutions**

Use defined substitutions from `docs/reuse/substitutions.txt`:
- `|ws_markup|` renders as Workshop (with `:program:` markup)
- `|sdk_markup|` renders as SDKcraft (with `:program:` markup)

**Reusable link references**

Common external URLs are defined in `docs/reuse/links.txt`:
- `` `GitHub`_ `` links to the Workshop repository
- `` `LXD`_ `` links to LXD documentation
- `` `SDKcraft`_ `` links to SDKcraft repository
- `` `Releases`_ `` links to Workshop releases page

These files are automatically included via the `docs/conf.py` configuration and are available in all reStructuredText documentation files. Using them ensures consistency and makes it easy to update URLs in a single location.

**Punctuation**

En dash (–): Use to represent a range or connection between two related items:

```
pages 10–15
East–West traffic
Ubuntu 22.04–24.04
```

Em dash (—): Avoid using em dashes. If possible, rephrase the sentence using other punctuation or sentence structure.

**Command line terminology**

Convention: Use [POSIX utility conventions](https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap12.html) when discussing command-line syntax, options, arguments, and other CLI elements.

---

## Project descriptions

**Short description (79 characters)**

```text
Workshop is a tool for defining and handling ephemeral development environments
```


**SDKcraft short description (77 characters)**

```text
SDKcraft is a tool that packages and publishes SDKs to be used with Workshop
```

---

## Documentation quality principles

**Clarity**

- State assumptions explicitly
- Define prerequisites clearly
- Avoid jargon without explanation
- Use consistent terminology

**Usability**

- Focus on actionable information
- Use direct imperatives for instructions
- Break complex tasks into clear steps
- Provide working examples

**Precision**

- Avoid ambiguous language
- Use exact commands and syntax
- Specify versions when relevant
- Maintain consistent structure

---

## Contributing

When contributing documentation:

1. Follow established patterns for file naming and structure
2. Use semantic line breaks in reStructuredText files
3. Include required metadata blocks
4. Add artefact markers for new concepts
5. Test examples before including them
6. Run documentation builds locally to verify

For detailed contribution guidelines, see {ref}`contributing` in the documentation.
