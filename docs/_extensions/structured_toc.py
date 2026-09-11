"""Compatibility adapter for section and external links in structured TOCs.

Keep upstream nodes, marker parsing, HTML rendering and ambiguity checks. Only
broaden the item grammar; let Sphinx resolve references (including CLI labels).
"""

import re

from docutils import nodes
from sphinx.addnodes import pending_xref
from sphinx_structured_toc.directives import SliceDirective
from sphinx_structured_toc.nodes import Domain, Slice, SliceItem
from sphinx_structured_toc.transforms import unique_id


class ReferenceSliceDirective(SliceDirective):
    def parse_item(self, line, lineno):
        text, mark_slice, mark_domain, error = self.parse_line(line)
        if error:
            return self.emit_error(error, lineno=lineno)

        expected = "slice item must contain exactly one :doc:, :ref:, or explicit external hyperlink"
        # Require explicit external link text; reject bare URLs and named links.
        external = re.fullmatch(r"`[^`<>]+\s+<https?://[^\s<>`]+>`__?", text)
        role = re.fullmatch(r":(?:doc|ref):`[^`]+`", text)
        if not (external or role):
            return self.emit_error(f"{expected}: {line!r}", lineno=lineno)

        parsed, messages = self.state.inline_text(text, lineno)
        links = [node for node in parsed if not isinstance(node, nodes.target)]
        if messages or len(links) != 1:
            return self.emit_error(f"{expected}: {line!r}", lineno=lineno)
        link = links[0]
        valid = (
            isinstance(link, pending_xref)
            and link.get("refdomain") == "std"
            and link.get("reftype") in {"doc", "ref"}
        ) or (external and isinstance(link, nodes.reference) and link.get("refuri"))
        if not valid:
            return self.emit_error(f"{expected}: {line!r}", lineno=lineno)
        if isinstance(link, pending_xref):
            # Missing destinations must fail even outside Sphinx nitpicky mode.
            link["refwarn"] = True

        item = SliceItem(rawsource=line)
        if mark_slice:
            item["mark_slice"] = True
        if mark_domain:
            item["mark_domain"] = True
        item.extend(parsed)
        return item


def prepare_output(app, doctree, docname):
    """Use heading-only ARIA targets and semantic lists for non-HTML output."""
    if app.builder.format == "html":
        used = {id_ for node in doctree.findall(nodes.Element) for id_ in node.get("ids", [])}
        for domain in doctree.findall(Domain):
            if domain.get("overridden"):
                continue
            section = domain.parent
            while section is not None and not isinstance(section, nodes.section):
                section = section.parent
            if section is None:
                continue  # Upstream already reports the missing heading.
            title = next(child for child in section if isinstance(child, nodes.title))
            # Upstream points at the entire section. Point at its title so the
            # accessible name contains only the heading, not every directory link.
            title_id = unique_id(f"{domain['section_id']}-heading", used)
            title["ids"].append(title_id)
            domain["section_id"] = title_id
        return

    # sphinx-llm builds Markdown twins with a separate builder. Convert only
    # presentation nodes after upstream resolution, preserving resolved links.
    for domain in list(doctree.findall(Domain)):
        listing = nodes.bullet_list()
        for slice_node in domain.findall(Slice):
            row = nodes.list_item()
            row += nodes.paragraph("", "", nodes.strong("", slice_node["name"]))
            links = nodes.bullet_list()
            for item in slice_node.children:
                entry = nodes.list_item()
                paragraph = nodes.paragraph()
                paragraph.extend(item.children)
                entry += paragraph
                links += entry
            row += links
            listing += row
        domain.replace_self(listing)


def setup(app):
    app.setup_extension("sphinx_structured_toc")
    app.add_directive("slice", ReferenceSliceDirective, override=True)
    # Upstream resolves names and checks ambiguity at the default priority 500.
    app.connect("doctree-resolved", prepare_output, priority=600)
    return {"version": "1.0", "parallel_read_safe": True, "parallel_write_safe": True}
