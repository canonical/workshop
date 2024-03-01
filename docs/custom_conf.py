import datetime

# Custom configuration for the Sphinx documentation builder.
# All configuration specific to your project should be done in this file.
#
# The file is included in the common conf.py configuration file.
# You can modify any of the settings below or add any configuration that
# is not covered by the common conf.py file.
#
# For the full list of built-in configuration values, see the documentation:
# https://www.sphinx-doc.org/en/master/usage/configuration.html

############################################################
### Project information
############################################################

# Product name
project = "Workshop"
author = "Canonical Group Ltd"
html_title = ""

# Uncomment if your product uses release numbers
# release = '1.0'

# The default value uses the current year as the copyright year
# To check the date when a GutHub repo was created:
# curl -H 'Authorization: token <OBTAINED HERE: https://github.com/settings/tokens>' \
#   -H 'Accept: application/vnd.github.v3.raw' \
#   https://api.github.com/repos/canonical/<REPO> | jq '.created_at'

copyright = "%s, %s" % (datetime.date.today().year, author)

## Open Graph configuration - defines what is displayed in the website preview
# The URL of the documentation output
ogp_site_url = "https://canonical-workshop.readthedocs-hosted.com/"
# The documentation website name (usually the same as the product name)
ogp_site_name = project
# An image or logo that is used in the preview
ogp_image = "https://assets.ubuntu.com/v1/253da317-image-document-ubuntudocs.svg"

# Update with the favicon for your product (default is the circle of friends)
html_favicon = ".sphinx/_static/favicon.png"

# (Some settings must be part of the html_context dictionary, while others
#  are on root level. Don't move the settings.)
html_context = {
    # Change to the link to your product website (without "https://")
    "product_page": "documentation.ubuntu.com",
    # Add your product tag to ".sphinx/_static" and change the path
    # here (start with "_static"), default is the circle of friends
    "product_tag": "_static/tag.png",
    # Change to the discourse instance you want to be able to link to
    # using the :discourse: metadata at the top of a file
    # (use an empty value if you don't want to link)
    "discourse": "https://discourse.ubuntu.com",
    # Change to the GitHub info for your project
    # Change to the Mattermost channel you want to link to
    # (use an empty value if you don't want to link)
    "mattermost": "https://chat.canonical.com/canonical/channels/SDK",
    "github_url": "https://github.com/canonical/workshop",
    # Change to the branch for this version of the documentation
    "github_version": "main",
    # Change to the folder that contains the documentation
    # (usually "/" or "/docs/")
    "github_folder": "/",
    # Change to an empty value if your GitHub repo doesn't have issues enabled.
    # This will disable the feedback button and the issue link in the footer.
    "github_issues": "enabled",
    # Controls the existence of Previous / Next buttons at the bottom of pages
    # Valid options: none, prev, next, both
    "sequential_nav": "none",
}

# Dropping the variant selector snippet:
# https://pradyunsg.me/furo/customisation/sidebar/
html_sidebars = {
    "**": [
        "sidebar/search.html",
        "sidebar/scroll-start.html",
        "sidebar/navigation.html",
        "sidebar/ethical-ads.html",
        "sidebar/scroll-end.html",
    ]
}

# If your project is on documentation.ubuntu.com, specify the project
# slug (for example, "lxd") here.
slug = ""

############################################################
### Redirects
############################################################

# Set up redirects (https://documatt.gitlab.io/sphinx-reredirects/usage.html)
# For example: 'explanation/old-name.html': '../how-to/prettify.html',

redirects = {}

############################################################
### Link checker exceptions
############################################################

# Links to ignore when checking links;
# the 'make linkcheck' target doesn't handle the anchors
# in Readthedocs.com's hosted documentation too well

linkcheck_ignore = [
    "http://127.0.0.1:8000",
    "https://github.com/canonical/workshop",
    "^https://.*\.readthedocs-hosted\.com/.*#\w+$",
]

# Pages on which to ignore anchors
# (This list will be appended to linkcheck_anchors_ignore_for_url)

custom_linkcheck_anchors_ignore_for_url = []

############################################################
### Additions to default configuration
############################################################

## The following settings are appended to the default configuration.
## Use them to extend the default functionality.

# Add custom Sphinx extensions as needed.
# This array contains recommended extensions that should be used.
# NOTE: The following extensions are handled automatically and do
# not need to be added here: myst_parser, sphinx_copybutton, sphinx_design,
# sphinx_reredirects, sphinxcontrib.jquery, sphinxext.opengraph
custom_extensions = [
    "sphinx_tabs.tabs",
    "canonical.youtube-links",
    "canonical.related-links",
    "canonical.custom-rst-roles",
    "canonical.terminal-output",
]

# Add custom required Python modules that must be added to the
# .sphinx/requirements.txt file.
# NOTE: The following modules are handled automatically and do not need to be
# added here: canonical-sphinx-extensions, furo, linkify-it-py, myst-parser,
# pyspelling, sphinx, sphinx-autobuild, sphinx-copybutton, sphinx-design,
# sphinx-reredirects, sphinx-tabs, sphinxcontrib-jquery, sphinxext-opengraph
custom_required_modules = []

# Add files or directories that should be excluded from processing.
custom_excludes = ["README.rst"]

# Add CSS files (located in .sphinx/_static/)
custom_html_css_files = []

# Add JavaScript files (located in .sphinx/_static/)
custom_html_js_files = []

# Add tags that you want to use for conditional inclusion of text
# (https://www.sphinx-doc.org/en/master/usage/restructuredtext/directives.html#tags)
custom_tags = []

## The following settings override the default configuration.

# Specify a reST string that is included at the end of each file.
# If commented out, use the default (which pulls the reuse/links.txt
# file into each reST file).
# custom_rst_epilog = ''

# Link Python config variables to reST replacements via rst_prolog
# https://www.sphinx-doc.org/en/master/usage/configuration.html#confval-rst_prolog
# (not so easy with reuse/substitutions.txt in default custom_rst_epilog)
rst_prolog = f"""
.. role:: center
   :class: align-center

.. |project| replace:: {project}
"""

# By default, the documentation includes a feedback button at the top.
# You can disable it by setting the following configuration to True.
disable_feedback_button = False

# Add tags that you want to use for conditional inclusion of text
# (https://www.sphinx-doc.org/en/master/usage/restructuredtext/directives.html#tags)
custom_tags = []

############################################################
### Additional configuration
############################################################

## Add any configuration that is not covered by the common conf.py file.

# Exclude command line output when copypasting samples
# (https://sphinx-copybutton.readthedocs.io/en/latest/use.html)
# Note that this explicitly violates the SG, but does so for a reason:
# https://docs.ubuntu.com/styleguide/en#code-examples-in-documentation
copybutton_prompt_text = "$ "
copybutton_here_doc_delimiter = "EOF"
copybutton_line_continuation_character = "\\"
