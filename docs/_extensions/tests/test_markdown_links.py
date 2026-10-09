"""Build-level regression tests; run with docs/.venv/bin/python -m unittest discover
-s docs/_extensions/tests. All fixtures and output stay outside the docs tree.
"""

from importlib.metadata import version
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from markdown_links import published_path  # noqa: E402

BASE = "https://example.com/docs"


@unittest.skipIf(int(version("sphinx-llm").split(".")[0]) >= 1,
                 "sphinx-llm 1.x resolves Markdown links itself")
class MarkdownLinksTests(unittest.TestCase):
    def build(self, extensions):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        source = root / "source"
        (source / "section").mkdir(parents=True)
        extension = Path(__file__).resolve().parents[1]
        # Like the sphinx-llm sub-build: the markdown builder comes from -b, not extensions.
        (source / "conf.py").write_text(
            f"import sys\nsys.path.insert(0, {str(extension)!r})\n"
            f"extensions = {extensions!r}\n"
            f"markdown_http_base = {BASE!r}\n"
            "master_doc = 'index'\n"
        )
        (source / "index.rst").write_text(
            "Home\n====\n\nSee :doc:`section/index` and :doc:`section/child`.\n\n"
            ".. toctree::\n\n   section/index\n"
        )
        (source / "section" / "index.rst").write_text(
            ".. _section-label:\n\nSection\n=======\n\n"
            "Back to :doc:`/index`; see :doc:`child`.\n\n"
            ".. toctree::\n\n   child\n"
        )
        (source / "section" / "child.rst").write_text(
            "Child\n=====\n\nUp to :doc:`index`, :ref:`section-label`, and :doc:`/index`.\n"
        )
        result = subprocess.run(
            [sys.executable, "-m", "sphinx", "-E", "-a", "-W", "--keep-going", "-q",
             "-b", "markdown", str(source), str(root / "out")],
            text=True, capture_output=True,
        )
        out = root / "out"
        pages = {path.relative_to(out).as_posix(): path.read_text()
                 for path in out.rglob("*.md")}
        return result.stderr.strip(), pages

    def test_section_index_links_use_published_paths(self):
        warnings, pages = self.build(["markdown_links"])
        self.assertEqual(warnings, "")
        self.assertIn(f"]({BASE}/section.md)", pages["index.md"])
        self.assertIn(f"]({BASE}/section/child.md)", pages["index.md"])
        self.assertIn(f"]({BASE}/index.md)", pages["section/index.md"])
        self.assertIn(f"]({BASE}/section/child.md)", pages["section/index.md"])
        self.assertIn(f"]({BASE}/section.md)", pages["section/child.md"])
        self.assertIn(f"]({BASE}/section.md#section-label)", pages["section/child.md"])
        self.assertIn(f"]({BASE}/index.md)", pages["section/child.md"])
        for name, markdown in pages.items():
            with self.subTest(page=name):
                self.assertNotIn("section/index.md", markdown)
                self.assertNotIn("docs//", markdown)

    def test_control_without_extension_links_index_md(self):
        # Proves the test above can fail; if this breaks, upstream fixed the links.
        warnings, pages = self.build([])
        self.assertEqual(warnings, "")
        self.assertIn(f"]({BASE}/section/index.md)", pages["section/child.md"])

    def test_published_path(self):
        cases = {
            "index.md": "index.md",
            "index.md#top": "index.md#top",
            "how-to/index.md": "how-to.md",
            "how-to/index.md#label": "how-to.md#label",
            "how-to/fix-workshops/index.md": "how-to/fix-workshops.md",
            "how-to/fix-workshops/purge.md": "how-to/fix-workshops/purge.md",
            "how-to/index-of-things.md": "how-to/index-of-things.md",
        }
        for path, expected in cases.items():
            with self.subTest(path=path):
                self.assertEqual(published_path(path), expected)


if __name__ == "__main__":
    unittest.main()
