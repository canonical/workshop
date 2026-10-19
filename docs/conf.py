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
# The documentation uses the Ulwazi theme to stay consistent and on brand
# with other Canonical documentation:
# https://github.com/canonical/ulwazi

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

# Repository with the documentation sources; used for the source and feedback links.
github_repo = "https://github.com/canonical/workshop"

# Suffix for the fallback view and edit source links.
default_source_extension = ".rst"

# Product website URL, without the 'https://' prefix.
product_page = "documentation.ubuntu.com"

# Dictionary of values to pass into the Sphinx context for all pages:
# https://www.sphinx-doc.org/en/master/usage/configuration.html#confval-html_context
html_context = {
    "product_page": product_page,
    "project": project,
    "author": author,
    "license": {
        "name": "CC-BY-SA 4.0",
        "url": "https://creativecommons.org/licenses/by-sa/4.0/",
    },
    "discourse": "https://discourse.ubuntu.com/",
    "matrix": "https://matrix.to/#/#documentation:ubuntu.com",
    "github_url": github_repo,
    "repo_folder": "/docs/",
    # Enables the feedback and source buttons, and the dark mode toggle.
    "feedback": True,
    "default_source_extension": default_source_extension,
    "default_edit_url": github_repo + "/edit/main/docs/index" + default_source_extension,
    "default_view_url": github_repo + "/blob/main/docs/index" + default_source_extension,
    "logo_link_URL": "https://documentation.ubuntu.com",
    "logo_img_URL": "https://assets.ubuntu.com/v1/82818827-CoF_white.svg",
    "logo_title": "Canonical",
    # Workshop's Google Tag Manager container; Ulwazi defaults to a different one.
    "tag_id": "GTM-KNX3CJC",
    "footer": {
        "product": True,
        "license": True,
        # The cookie banner script (_static/js/bundle.js) binds to this link.
        "entries": [
            '<a class="js-revoke-cookie-manager" href="#tracker-settings">Manage your tracker settings</a>',
        ],
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

html_theme = "ulwazi"
html_static_path = ["_static"]

html_css_files = [
    "workshop.css",
    "flat-toctree.css",
    "cookie-banner.css",
    # "category-nav.css",  # category_nav extension disabled on this branch
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

# Generate an 'llms-full.txt' file containing all of the documentation content
llms_txt_full_build = True

# Suppress warnings from upstream markdown builder
llms_txt_suppress_unknown_node_warnings = True

# Publish each page's Markdown twin at its URL plus '.md', as advertised in
# reference/ai-agents.rst. sphinx-llm 1.1 deprecates the mode in favor of
# "append", which publishes section indexes under a different path.
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
# NOTE: "ulwazi" must precede "sphinx_modern_pdf_style"; the theme injects the
#       PDF branding defaults that the PDF style reads.
extensions = [
    "ulwazi",
    "sphinx_modern_pdf_style",
    "myst_parser",
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
    "sphinx_structured_toc",
    "structured_toc",
    # "category_nav",  # disabled on this branch; extension kept in _extensions/
]

exclude_patterns = [
    "doc-cheat-sheet*",
    ".venv*",
    "_dev",
    "readme.rst",
    "reference/cli/sdk-*.rst",
    "reference/cli/workshop-*.rst",
    "reference/cli/sdkcraft-*.rst",
    "reference/definition-files/_interfaces/*",
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

# MyST syntax extensions for the Markdown pages; the release notes rely on
# linkify for bare URLs.
myst_enable_extensions = ["substitution", "deflist", "linkify"]

# Don't publish reST sources alongside the HTML pages.
html_copy_source = False
html_show_sourcelink = False

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

# Apply the Canonical PDF style to the Read the Docs PDF build.
set_modern_pdf_config = True


def setup(app):
    # Under dirhtml, Sphinx builds each page URL with a ".html" suffix that the
    # site doesn't serve. Furo used to rewrite it, and Ulwazi doesn't, which
    # breaks the canonical link. Use the directory URL that dirhtml actually
    # serves instead.
    def _fix_dirhtml_pageurl(app, pagename, templatename, context, doctree):
        pageurl = context.get("pageurl")
        if app.builder.name == "dirhtml" and pageurl and pageurl.endswith(".html"):
            context["pageurl"] = app.config.html_baseurl + app.builder.get_target_uri(pagename)

    app.connect("html-page-context", _fix_dirhtml_pageurl)

    # With html_title = "", Ulwazi sets the "sidebar_hide_name" theme option,
    # which it doesn't support, and the strict build fails on the resulting
    # warning. Drop the option after Ulwazi's own config-inited handler runs.
    def _drop_sidebar_hide_name(app, config):
        config.html_theme_options.pop("sidebar_hide_name", None)

    app.connect("config-inited", _drop_sidebar_hide_name, priority=600)
