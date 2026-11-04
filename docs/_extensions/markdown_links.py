"""Sphinx extension to link Markdown exports to their published files.

With the dirhtml builder and llms_txt_suffix_mode = "url-suffix", sphinx-llm
0.4 publishes a section index such as 'how-to/index' as 'how-to.md', but
sphinx-markdown-builder links it as 'how-to/index.md', which does not exist.
sphinx-llm 1.0 resolves links against the published layout itself, so remove
this extension once the Sphinx Stack moves to sphinx-llm 1.x.
"""

from importlib.metadata import version
import posixpath

from sphinx_markdown_builder.translator import MarkdownTranslator


def published_path(path):
    """Map a built Markdown path to the path sphinx-llm publishes it at.

    Args:
        path: Path like 'how-to/index.md#label' or 'how-to/develop-sdks/build-an-sdk.md'

    Returns:
        Path like 'how-to.md#label'; the root 'index.md' and other pages are unchanged
    """
    target, hashmark, fragment = path.partition('#')
    directory, filename = posixpath.split(target)
    if directory and filename == 'index.md':
        target = directory + '.md'
    return target + hashmark + fragment


class PublishedLinkTranslator(MarkdownTranslator):
    """Markdown translator that rewrites absolute links to section indexes."""

    def _adjust_url(self, url):
        adjusted = super()._adjust_url(url)
        prefix = f"{self.config.markdown_http_base}/"
        if self.config.markdown_http_base and adjusted.startswith(prefix):
            return prefix + published_path(adjusted[len(prefix):])
        return adjusted


def setup(app):
    # sphinx-llm 1.x ships its own Markdown translator; don't replace it.
    if int(version('sphinx-llm').split('.')[0]) < 1:
        app.set_translator('markdown', PublishedLinkTranslator)
    return {"version": "1.0", "parallel_read_safe": True, "parallel_write_safe": True}
