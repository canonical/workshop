#!/usr/bin/env python3

import os
import sys
import argparse
import yaml
import shutil
import tempfile
from pathlib import Path
from git import Repo


def load_manifest(path):
    """
    Loads the YAML manifest file and returns it as a dictionary.
    """
    with open(path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def shallow_clone(repo_url, branch, clone_dir):
    """
    Performs a shallow clone of the given branch into clone_dir using GitPython.
    """
    return Repo.clone_from(
        repo_url, clone_dir, branch=branch, multi_options=["--depth=1"]
    )


def copy_docs_for_top_dirs(clone_dir, prefix, branch_name, top_dirs):
    """
    For each directory in top_dirs, copies <top_dir>/docs/ to
    <prefix>/<branch_name>/<top_dir>/, removing <top_dir> if it exists.
    """
    for top_dir in top_dirs:
        docs_src = Path(clone_dir) / top_dir / "docs"
        branch_dir = Path(prefix) / branch_name
        docs_dst = branch_dir / top_dir

        if docs_src.is_dir():
            if docs_dst.exists():
                shutil.rmtree(docs_dst)
            shutil.copytree(docs_src, docs_dst)
            print(f"Copied {top_dir}/docs -> {docs_dst}")
        else:
            print(f"No docs/ found for {top_dir} on branch {branch_name}. Skipping.")


def main():
    parser = argparse.ArgumentParser(
        description="Pull remote SDK docs from a single Git repository."
    )
    parser.add_argument(
        "--manifest",
        default="remote-sdks.yaml",
        help="Path to the YAML manifest file (default: remote-sdks.yaml).",
    )
    args = parser.parse_args()

    manifest_path = args.manifest
    if not os.path.isfile(manifest_path):
        print(f"Error: manifest file '{manifest_path}' not found.")
        sys.exit(1)

    manifest = load_manifest(manifest_path)
    prefix = manifest.get("prefix", "")
    repo_url = manifest.get("repo_url")
    if not repo_url:
        print("Error: 'repo_url' not specified in manifest.")
        sys.exit(1)

    branches = manifest.get("branches", [])
    if not branches:
        print("No branches specified in the manifest. Nothing to do.")
        sys.exit(0)

    for branch_info in branches:
        branch_name = branch_info.get("name")
        top_dirs = branch_info.get("top_dirs", [])
        if not branch_name:
            print("Warning: Branch entry without a 'name'. Skipping.")
            continue

        with tempfile.TemporaryDirectory(prefix=f"clone_{branch_name}_") as clone_dir:
            print(f"\n=== Processing branch: {branch_name} ===")
            shallow_clone(repo_url, branch_name, clone_dir)

            branch_path = Path(prefix) / branch_name
            branch_path.mkdir(parents=True, exist_ok=True)

            copy_docs_for_top_dirs(clone_dir, prefix, branch_name, top_dirs)
            print(f"Done with branch '{branch_name}'.")

    print("\nAll branches processed successfully!")


if __name__ == "__main__":
    main()
