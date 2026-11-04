// Make toctree entries with :class: flat-toctree non-clickable containers in sidebar
// This script uses pre-computed absolute docnames from the Sphinx build
// Also makes the immediate parent of the current page non-clickable if it has children
document.addEventListener('DOMContentLoaded', function() {
    // Check if flat-toctree docnames were injected by the Sphinx extension
    if (!window.FLAT_TOCTREE_PATHS || !Array.isArray(window.FLAT_TOCTREE_PATHS)) {
        return;
    }

    // Convert docnames to a Set for O(1) lookup
    var docnameSet = new Set(window.FLAT_TOCTREE_PATHS);

    // Current page's project-relative URL directory, injected by the Sphinx extension.
    // Robust against URL prefixes like /latest/, /en/latest/, custom domains, etc.
    var currentPageDir = (typeof window.FLAT_TOCTREE_PAGE_DIR === 'string')
        ? window.FLAT_TOCTREE_PAGE_DIR
        : '';

    // Resolve a sidebar href to an absolute docname, anchored on the current page directory.
    function resolveHrefToDocname(href, currentPageDir) {
        // Remove leading './'
        href = href.replace(/^\.\//, '');

        // Remove trailing slash
        if (href.endsWith('/')) {
            href = href.slice(0, -1);
        }

        // Remove trailing '/index.html' or '/index'
        href = href.replace(/\/index(\.html)?$/, '');

        // If it starts with '../', resolve relative to current document
        if (href.startsWith('../')) {
            var currentParts = currentPageDir.split('/');
            var hrefParts = href.split('/');

            for (var i = 0; i < hrefParts.length; i++) {
                if (hrefParts[i] === '..') {
                    if (currentParts.length > 0) {
                        currentParts.pop();
                    }
                } else if (hrefParts[i] === '.') {
                    continue;
                } else if (hrefParts[i]) {
                    currentParts.push(hrefParts[i]);
                }
            }

            return currentParts.join('/');
        }

        // Simple relative path (no ../, no leading /): join with the current page directory.
        // E.g. on /how-to/ (currentPageDir='how-to'), href 'customize-workshops/' -> 'how-to/customize-workshops'.
        if (!href.startsWith('/') && href.indexOf('../') === -1) {
            if (currentPageDir) {
                return currentPageDir + '/' + href;
            }
            return href;
        }

        // Otherwise return as-is (absolute path)
        return href;
    }

    // Find the current page link in the sidebar
    var currentPageLi = document.querySelector('#drawer li.current-page');

    // Also make the immediate parent of the current page non-clickable if it has children
    if (currentPageLi) {
        var parentUl = currentPageLi.parentElement;
        if (parentUl && parentUl.tagName === 'UL') {
            var parentLi = parentUl.parentElement;
            if (parentLi && parentLi.tagName === 'LI' && parentLi.classList.contains('has-children')) {
                parentLi.classList.add('flat-toctree');
            }
        }
    }

    // Only check sidebar links (more efficient than all links)
    var sidebarLinks = document.querySelectorAll('#drawer .p-side-navigation__list a[href]');

    sidebarLinks.forEach(function(link) {
        var href = link.getAttribute('href');
        if (!href) return;

        // Convert href to absolute docname
        var docname = resolveHrefToDocname(href, currentPageDir);

        if (docnameSet.has(docname)) {
            // This link should be non-clickable
            var li = link.closest('li.has-children');
            if (li) {
                li.classList.add('flat-toctree');
            }
        }
    });
});

