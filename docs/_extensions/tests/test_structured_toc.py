"""Build-level regression tests; run with docs/.venv/bin/python -m unittest discover
-s docs/_extensions/tests. All fixtures and output stay outside the docs tree.
"""

from collections import Counter
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest

from bs4 import BeautifulSoup


class StructuredTocTests(unittest.TestCase):
    def build(self, items, extra="", builder="dirhtml"):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        source = root / "source"
        source.mkdir()
        extension = Path(__file__).resolve().parents[1]
        (source / "conf.py").write_text(
            f"import sys\nsys.path.insert(0, {str(extension)!r})\n"
            "extensions = ['sphinx_structured_toc', 'structured_toc', 'sphinx_markdown_builder']\n"
            "master_doc = 'index'\n"
        )
        (source / "index.rst").write_text(
            "Directory\n=========\n\nSubject\n-------\n\n.. domain::\n\n"
            "   .. slice:: Resources\n\n"
            + "\n".join("      " + item for item in items)
            + "\n\n" + extra
            + "\n.. toctree::\n   :hidden:\n\n   page\n"
        )
        (source / "page.rst").write_text(
            "Page\n====\n\n.. _section-label:\n\nEmbedded command\n----------------\n\nDetails.\n"
        )
        result = subprocess.run(
            [sys.executable, "-m", "sphinx", "-E", "-a", "-W", "--keep-going", "-q",
             "-b", builder, str(source), str(root / "out")],
            text=True, capture_output=True,
        )
        relevant = result.stderr
        html = root / "out" / "index.html"
        if builder == "markdown":
            return relevant.strip(), (root / "out" / "index.md").read_text()
        return relevant.strip(), BeautifulSoup(html.read_text(), "html.parser") if html.exists() else None

    def test_supported_links_context_and_ids(self):
        warnings, page = self.build([
            ':doc:`Page <page>`', ':ref:`Command <section-label>` slice domain',
            '`Repository <https://example.com/repo>`__ slice',
            '`Named external <https://example.com/named>`_ domain',
        ])
        self.assertEqual(warnings, "")
        nav = page.select_one("nav.domain-list")
        self.assertEqual([a["href"] for a in nav.select("a")],
                         ["page/", "page/#section-label", "https://example.com/repo", "https://example.com/named"])
        ids = Counter(node["id"] for node in page.select("[id]"))
        self.assertTrue(all(count == 1 for count in ids.values()))
        for node in page.select("[aria-labelledby]"):
            for target in node["aria-labelledby"].split():
                self.assertEqual(ids[target], 1)
        self.assertEqual(page.find(id=nav["aria-labelledby"]).name, "h2")
        command = nav.select("a")[1]
        targets = [page.find(id=target).get_text() for target in command["aria-labelledby"].split()]
        self.assertEqual(targets[:2], ["Command", "Resources"])
        self.assertTrue(targets[2].startswith("Subject"))

    def test_markdown_preserves_headings_slices_and_links(self):
        warnings, markdown = self.build([
            ':doc:`Page <page>`', ':ref:`Command <section-label>` slice domain',
            '`Repository <https://example.com/repo>`__',
        ], builder="markdown")
        self.assertEqual(warnings, "")
        for text in ["Subject", "**Resources**", "[Page](page.md)",
                     "[Command](page.md#section-label)", "[Repository](https://example.com/repo)"]:
            self.assertIn(text, markdown)

    def test_malformed_items(self):
        for item in ["plain text", ':ref:`section-label` trailing',
                     ':doc:`page` :ref:`section-label`', ':ref:`section-label` slice slice',
                     ':ref:`section-label` domain domain', ':any:`page`',
                     'https://example.com', '`Repository <javascript:alert(1)>`__',
                     ':ref:`unterminated']:
            with self.subTest(item=item):
                warnings, _ = self.build([item])
                self.assertRegex(warnings, "slice item|trailing token")

    def test_unresolved_references(self):
        for item in [':doc:`absent`', ':ref:`absent`']:
            with self.subTest(item=item):
                warnings, _ = self.build([item])
                self.assertRegex(warnings, "not found|undefined label|unknown document")

    def test_ambiguity_checks_remain_enabled(self):
        extra = ".. domain::\n\n   .. slice:: Other\n\n      :doc:`Same <page>`\n"
        warnings, _ = self.build([':doc:`Same <page>`'], extra)
        self.assertIn("ambiguous link text", warnings)
        warnings, _ = self.build([':doc:`Same <page>` slice'], extra)
        self.assertEqual(warnings, "")


if __name__ == "__main__":
    unittest.main()
