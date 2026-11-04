---
applyTo: "docs/**/*.rst,docs/**/*.md"
description: Documentation file conventions for Workshop.
---

# Documentation Instructions

## Primary Reference

**Always consult**: [`docs/doc-style-guide.md`](../../docs/doc-style-guide.md) (1138 lines, comprehensive)

## Quick Checks (Top Issues)

### File Naming

Lowercase with dashes: `connect-vscode.rst` ✅ | `ConnectVSCode.rst` ❌

### Metadata Block

Required immediately after anchor label:
```restructuredtext
.. _how_add_actions:

.. meta::
   :description: Brief description for search engines and social media.
```

### Semantic Line Breaks

One line per clause:
```restructuredtext
Install Workshop,
upgrading the prerequisites if needed,
then ensure it runs.
```

### Product Names

- Use substitutions: `|ws_markup|` (renders `:program:`Workshop``)
- Capitalize: Workshop, SDKcraft, LXD

### Code Examples

```restructuredtext
.. code-block:: console

   $ workshop launch dev

     Launched dev
```
- Use `console` lexer
- `$` prompt (non-selectable)
- Indent output with two spaces

## Gold Standard Examples

- Tutorial: [`docs/tutorial/part-1-get-started.rst`](../../docs/tutorial/part-1-get-started.rst)
- Explanation: [`docs/explanation/index.rst`](../../docs/explanation/index.rst)
- Reference: [`docs/reference/cli/workshop-launch.rst`](../../docs/reference/cli/workshop-launch.rst)
