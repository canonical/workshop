import datetime
import os
import sys
import textwrap

# Add _extensions directory to Python path for custom extensions
sys.path.insert(0, os.path.abspath('_extensions'))

# Configuration for the Sphinx documentation builder.
# All configuration specific to your project should be done in this file.
#
# A complete list of built-in Sphinx configuration values:
# https://www.sphinx-doc.org/en/master/usage/configuration.html
#
# The Sphinx Stack uses the Canonical Sphinx theme to keep all documentation consistent
# and on brand:
# https://github.com/canonical/canonical-sphinx

#######################
# Project information #
#######################

project = "Workshop"
author = "Canonical Ltd."
copyright = f"{datetime.date.today().year}"

# Sidebar documentation title; empty to defer to the theme default.
html_title = ""

# Documentation website URL
ogp_site_url = "https://ubuntu.com/workshop/docs/"

# Preview name of the documentation website
ogp_site_name = project

# Preview image URL
ogp_image = "https://assets.ubuntu.com/v1/253da317-image-document-ubuntudocs.svg"

# Product favicon; shown in bookmarks, browser tabs, etc.
html_favicon = "_static/favicon.png"

# Dictionary of values to pass into the Sphinx context for all pages:
# https://www.sphinx-doc.org/en/master/usage/configuration.html#confval-html_context
html_context = {
    "product_page": "documentation.ubuntu.com",
    "product_tag": "_static/tag.png",
    "discourse": "https://discourse.ubuntu.com/",
    "matrix": "https://matrix.to/#/#documentation:ubuntu.com",
    "github_url": "https://github.com/canonical/workshop",
    "repo_default_branch": "main",
    "repo_folder": "/docs/",
    "display_contributors": False,
    "github_issues": "enabled",
    "author": author,
    "license": {
        "name": "CC-BY-SA 4.0",
        "url": "https://creativecommons.org/licenses/by-sa/4.0/",
    },
}

# Project slug; see https://meta.discourse.org/t/what-is-category-slug/87897
slug = 'workshop/docs'

#######################
# Sitemap configuration: https://sphinx-sitemap.readthedocs.io/
#######################

html_baseurl = f"https://ubuntu.com/workshop/docs/"
sitemap_url_scheme = "{link}"
sitemap_show_lastmod = True
sitemap_excludes = [
    "404/",
    "genindex/",
    "search/",
]
sitemap_filename = "doc-sitemap.xml"

################################
# Template and asset locations #
################################

html_static_path = ["_static"]
templates_path = ["_templates"]

html_css_files = [
    "workshop.css",
    "flat-toctree.css",
    "category-nav.css",
    "cookie-banner.css",
]

html_js_files = [
    "flat-toctree.js",
    "js/bundle.js",
    "js/overwrite_links.js",
]

#############
# Redirects #
#############

# Add redirects to the 'redirects.txt' file
# https://sphinxext-rediraffe.readthedocs.io/en/latest/
rediraffe_redirects = "redirects.txt"

# Strips '/index.html' from destination URLs when building with 'dirhtml'
rediraffe_dir_only = True

############################
# sphinx-llm configuration #
############################

llms_txt_description = textwrap.dedent(
    """\
    This is the documentation for Workshop, a Canonical tool for defining and
    handling ephemeral development environments that run as containers.
    List your dependencies and components in YAML to define an environment,
    composed of SDKs: independent but connectable units of functionality
    from the SDK Store or project repo. Workshop targets teams who build and
    maintain complex, error-prone workspaces in domains
    such as AI/ML, robotics, IoT, and EdTech.
    """
)

llms_txt_suffix_mode = "url-suffix"

# The base URL for references built by sphinx-markdown-builder.
if os.environ.get("READTHEDOCS"):
    markdown_http_base = html_baseurl

###########################
# Link checker exceptions #
###########################
# NOTE: Project uses lychee (lychee.toml) for link checking, not Sphinx's
# built-in linkcheck. These settings only apply when running `sphinx-build -b linkcheck`.

linkcheck_ignore = [
    "http://127.0.0.1:8000",
    "https://github.com",
    r"https://matrix\.to/.*",
    "https://example.com",
    r"https://.*\.sourceforge\.(net|io)/.*",
]

linkcheck_anchors_ignore_for_url = [r"https://github\.com/.*"]
linkcheck_retries = 3

########################
# Configuration extras #
########################

# Custom Sphinx extensions; see
# https://www.sphinx-doc.org/en/master/usage/extensions/index.html
extensions = [
    "canonical_sphinx",
    "notfound.extension",
    "sphinx_design",
    "sphinx_rerediraffe",
    "sphinxcontrib.jquery",
    "sphinxcontrib.mermaid",
    "sphinxext.opengraph",
    "sphinx_config_options",
    "sphinx_llm.txt",
    "sphinx_related_links",
    "sphinx_roles",
    "sphinx_terminal",
    "sphinx_youtube_links",
    "sphinxcontrib.cairosvgconverter",
    "sphinx_sitemap",
    "flat_toctree",
    "category_nav",
]

exclude_patterns = [
    "doc-cheat-sheet*",
    ".venv*",
    "readme.rst",
    "reference/cli/sdk-*.rst",
    "reference/cli/workshop-*.rst",
    "reference/cli/sdkcraft-*.rst",
    "coverage.md",
    "examples/*",
]

# Inlined from former docs/reuse/links.txt and docs/reuse/substitutions.txt.
rst_epilog = """
.. _Canonical website: https://canonical.com/
.. _GitHub: https://github.com/canonical/workshop/
.. _LXC: https://canonical.com/lxd/docs/latest/explanation/lxd_lxc/
.. _LXD: https://canonical.com/lxd/docs/latest/
.. _SDKcraft: https://github.com/canonical/sdkcraft/
.. _Releases: https://github.com/canonical/workshop/releases/

.. |ws_markup| replace:: :program:`Workshop`
.. |sdk_markup| replace:: :program:`SDKcraft`
"""

# Specifies a reST snippet to be prepended to each .rst file.
# This defines a :center: role that centers table cell content.
# This defines a :h2: role that styles content for use with PDF generation.
rst_prolog = """
.. role:: center
   :class: align-center
.. role:: h2
    :class: hclass2
.. role:: woke-ignore
    :class: woke-ignore
.. role:: vale-ignore
    :class: vale-ignore
"""

# Workaround for https://github.com/canonical/canonical-sphinx/issues/34
if "discourse_prefix" not in html_context and "discourse" in html_context:
    html_context["discourse_prefix"] = html_context["discourse"] + "/t/"

copybutton_prompt_text = "$ "
copybutton_here_doc_delimiter = "EOF"
copybutton_line_continuation_character = "\\"

# Let rendered Mermaid diagrams size to their natural aspect ratio instead of
# the extension's fixed 500px height, which letterboxes wide diagrams with
# vertical whitespace. Fullscreen view overrides this with !important.
mermaid_height = "auto"

# Disable Mermaid's useMaxWidth so each diagram carries explicit pixel
# dimensions. Otherwise the extension stylesheet stretches every diagram to the
# full content width and, with mermaid_height = "auto" above, narrow state
# diagrams render many times too tall. The matching SVG sizing override lives in
# _static/workshop.css. Keep startOnLoad False; the extension renders manually.
mermaid_init_config = {
    "startOnLoad": False,
    "flowchart": {"useMaxWidth": False},
    "sequence": {"useMaxWidth": False},
    "state": {"useMaxWidth": False},
}

# https://www.sphinx-doc.org/en/master/usage/configuration.html#confval-smartquotes_action
smartquotes_action = "qe"


# Expose the sphinx_llm "llms_txt_enabled" flag to the HTML templates so the
# per-page Markdown <link rel="alternate"> in _templates/base.html is emitted
# only when the Markdown twins are actually generated. Reads app.config so the
# Makefile autobuild override (-D=llms_txt_enabled=0) is respected.
def setup(app):
    def _expose_llms_flag(app, pagename, templatename, context, doctree):
        context["llms_txt_enabled"] = getattr(app.config, "llms_txt_enabled", True)

    app.connect("html-page-context", _expose_llms_flag)
