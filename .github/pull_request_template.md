# Description

# Self-review quick check

 * [ ] Make decisions that cost a lot to reverse explicit in the PR description.
 * [ ] Avoid nested conditions.
 * [ ] Delete dead code and redundant comments.
 * [ ] Normalise symmetries by sticking to doing identical things identically. 
 ```
 // one way to handle errors
 if err := f(); err != nil {
    ...
 }
 
 // one way to handle multiple returns
 val, err := f()
 if err != nil {
    ...
 }
 ...
 ```
 * [ ] Check that coupled code elements, files, and directories are adjacent. For example, test data is stored as close as possible to a test.
 * [ ] Put variable declaration and initialisation together.
 * [ ] Divide large expressions into digestable and self-explanatory ones. Use multiple variables if required.
 * [ ] Put a blank line between two logically different chunks of code.
 * [ ] Follow the [style guide](https://github.com/canonical/workshop/tree/main/docs/coding-style-guide.md#error-handling) for new error messages.

## Docs

Procedure:

* [ ] I have checked and added or updated relevant documentation.
* [ ] I have checked and added or updated relevant release notes.
* [ ] I have included the technical author in the review.

Content:

* [ ] Headings and titles accurately describe the content.
* [ ] New and updated pages include correct metadata.
* [ ] Documentation tests are added or updated where applicable (for `tutorial/` and `how-to/` sections).
* [ ] Documentation follows the [style guide](https://github.com/canonical/workshop/tree/main/docs/doc-style-guide.md).

---

Or:

* [ ] I confirm the PR has no implications for documentation.
