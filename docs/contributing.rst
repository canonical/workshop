.. _contributing:

How to contribute
=================

We believe everyone has something valuable to contribute,
whether you're a coder, a writer or a tester.
Here's how and why you could get involved:

- **Why join us**:
  Work with like-minded people, grow your skills,
  connect with diverse professionals and make a difference.

- **What do you get**:
  Personal growth, recognition for your contributions,
  early access to new features and the joy of seeing your work appreciated.

- **Start early, start simple**:
  Dive into code contributions,
  improve documentation or be among the first testers.
  Your presence matters, regardless of experience or the scale of your input.

The guidelines below will keep your contributions effective and meaningful.


Environment setup
-----------------

.. @artefact workshopd
.. @artefact API
.. @artefact workshopd run

#. ``Workshop`` has a client-server architecture.
   Its ``workshopd`` daemon exposes a RESTful API (see :file:`internal/daemon/api.go`) to the clients.
   To run the daemon locally:

   .. code-block:: console

      go install ./...
      export WORKSHOP=~/workshop
      export WORKSHOP_DEBUG=1
      workshopd run --create-dirs

   The client can connect using the daemon's Unix domain socket:

   .. code-block:: console


      export WORKSHOP=~/workshop
      workshop list

#. ``Spread`` is the end-to-end testing tool for ``workshop``.
   Install it from `our custom fork <https://github.com/dmitry-lyfar/spread>`_:

   .. code-block:: console

      git clone https://github.com/dmitry-lyfar/spread
      cd spread
      go install ./...


   Make sure the ``$GOPATH/bin`` directory is included in ``$PATH``.
   After successful installation, you should see the help message by running:

   .. code-block:: console

      spread -h


Coding
------

In Workshop, commit messages differ from conventional commits in capitalisation:

.. code-block:: none

   Ensure correct permissions and ownership for the content mounts

    * Work around an LXD issue regarding empty dirs:
      https://github.com/canonical/lxd/issues/12648

    * Ensure the source directory is owned by the user running a workshop.

   Links:
   - ...
   - ...


The messages rarely, if ever, state the type of the commit
(e.g. ``fix``, ``feat``, etc.);
these are used for branch naming, for example:

- ``canonical/feat/workspace-start``
- ``canonical/fix/spread-tests-github``
- ``canonical/chore/update-lxd``


Commits that focus on docs must use the ``Doc:`` type prefix
with an optional scope in square brackets:

.. code-block:: none

   Doc[chore]: Align references


PR descriptions should follow the PR template checklist,
which largely reiterates this section.


After receiving review comments,
optimise for commit history clarity.
Address review comments with 
`fixup commits <https://git-scm.com/docs/git-commit/2.32.0#Documentation/git-commit.txt---fixupamendrewordltcommitgt>`_ 
and rebase using 
`autosquash <https://git-scm.com/docs/git-rebase#Documentation/git-rebase.txt---autosquash>`_ 
when reasonable.


Reversibility
~~~~~~~~~~~~~

When making decisions that might be costly to reverse,
explicitly state the rationale in the PR description.
This helps to understand the reasoning and collaborate better.


Coding standards
~~~~~~~~~~~~~~~~

- **Avoid nested conditions**:
  Refrain from nesting conditions to enhance readability and maintainability.

- **Eliminate dead code and redundant comments**:
  Remove unused or obsolete code and comments.
  This promotes a cleaner code base and reduces confusion.

- **Normalise symmetries**:
  Handle identical operations consistently, using a uniform approach.
  This also improves consistency and readability.


Error handling
~~~~~~~~~~~~~~

When handling errors or multiple returns,
follow a consistent pattern:

.. code-block:: go

   // one way to handle errors
   if err := f(); err != nil {
      ...
   }

   // one way to handle multiple returns
   val, err := f()
   if err != nil {
      ...
   }


Error messages
~~~~~~~~~~~~~~

- **Be consistent**:
  Try to match the style of existing error messages.
  Most of these can be found by searching for ``fmt.Errorf`` and ``errors.New``.
  Paths and other identifiers should be double-quoted if possible.

- **Consider the user experience**:
  Error messages should be clear and actionable.

- **Be specific**:
  For example, if a file was not found, the error message should include its path.

- **Mind the nesting**:
  Start in lowercase and avoid trailing punctuation.
  Avoid excessively long or repetitive error chains.
  A common template is: ``what was attempted: why it went wrong``.


Code structure
~~~~~~~~~~~~~~

- **Check coupled code elements**:
  Verify that coupled code elements, files and directories are adjacent.
  For instance, store test data close to the corresponding test code.

- **Group variable declaration and initialisation**:
  Declare and initialise variables together
  to improve code organisation and readability.

- **Divide large expressions**:
  Break down large expressions
  into smaller self-explanatory parts.
  Use multiple variables if necessary
  to make the code more understandable
  and choose names to reflect their purpose.

- **Use blank lines for logical separation**:
  Insert a blank line between two logically distinct sections of code.
  This improves its structure and makes it easier to comprehend.


Testing
-------

Make sure to run unit and integration tests before submitting a PR.
We use a ``go test``-compatible
`gocheck <https://pkg.go.dev/gopkg.in/check.v1#section-readme>`_:

.. code-block:: console

   go test ./...
   go test -check.f <TestName|SuiteName>


To run end-to-end tests and integration tests with
`our custom fork <https://github.com/dmitry-lyfar/spread>`_
of ``Spread``:

.. code-block:: console

   spread tests/<TestPathName>


To check code coverage:

.. code-block:: console

   go test --coverpkg=<./...|package> covermode=<set|count|atomic> -coverprofile=<OutputFile> <./...|package>

   // For example to measure coverage using all tests:
   go test covermode=count -coverprofile=cover.out ./...


To generate an HTML representation:

.. code-block:: console

   go tool cover -html=<OutputFile> -o <OutputHTML>

   // For example:
   go tool cover -html=cover.out -o cover.html

   // The output flag can be omitted to open in the default browser:
   go tool cover -html=cover.out


The above will work for unit and integration tests instrumented directly with
`go test`. Integration tests run using `spread` will create the coverprofile
automatically, however the artifacts will need to be collected from the VM.
This can be accomplished by using the `-artifacts` flag when running `spread`.

.. code-block:: console

   spread -artifacts=<path-to-dest> tests/integration/


How to run a local SDK Store
----------------------------

.. @artefact SDK
.. @artefact sdkcraft (CLI)
.. @artefact SDK Store

To test SDKs with |ws_markup| locally without publishing,
it is possible to run a local instance of SDK Store.
This guide uses the open-source `fake-gcs-server <https://github.com/fsouza/fake-gcs-server>`_.

.. note::

   This guide assumes you're familiar with :ref:`SDKcraft <how_use_sdkcraft>`.


Create the directory structure
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

SDK Store relies on a directory structure
to determine SDK names and channels.
Therefore, when running a store locally,
the directory structure must mirror that of the real store.

The 'fake store' directory can be named as preferred;
however, the remainder of the structure and naming convention is mandatory.

.. code-block:: console

   $ mkdir -p fake-store/sdk-store/<SDK>/<RELEASE>/<CHANNEL>

Here:

.. @artefact SDK name

- :samp:`<SDK>` is the SDK name (e.g. :samp:`my-sdk`)

- :samp:`<RELEASE>` is the SDK release (e.g. :samp:`latest`)

.. @artefact SDK channel

- :samp:`<CHANNEL>` is the SDK channel (e.g. :samp:`edge`)


Copy the SDK
~~~~~~~~~~~~

.. @artefact SDK definition

Place the SDK files in the deepest directory from the previous step
(e.g. :file:`fake-store/sdk-store/my-sdk/latest/edge/my-sdk/`).
Rename the SDK definition (e.g. :file:`my-sdk.yaml`) to :file:`sdk.yaml`
and place it at the same nesting level:

.. code-block:: console

   $ ls fake-store/sdk-store/my-sdk/latest/edge

     my-sdk.sdk  sdk.yaml


Run the local store
~~~~~~~~~~~~~~~~~~~

Pass the top-level SDK store directory to this :command:`go run` command:

.. code-block:: console

   $ go run github.com/fsouza/fake-gcs-server@latest \
     -data fake-store/ \
     -filesystem-root fake-store/ \
     -scheme http -port 8080 \
     -public-host localhost:8080

     time=1990-01-01T00:00:00.000+00.00 level=INFO msg="server started at http://0.0.0.0:8080"


Use the local store with |ws_markup|
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

To override the URL that |ws_markup| uses to connect to SDK Store,
configure the |ws_markup| snap
with the address from :option:`!-public-host` in the step above,
adding :samp:`/storage/v1/` as the path:

.. code-block:: console

   $ sudo snap set workshop store.url=http://localhost:8080/storage/v1/
   $ sudo snap restart workshop


|ws_markup| will now use the local store.


Revert changes
~~~~~~~~~~~~~~

To go back to the default store:

.. code-block:: console

   $ sudo snap set workshop store.url=""


|ws_markup| will now use the default URL.

Documentation
-------------

All documentation resides in the ``docs/`` directory.
To build and run it at ``127.0.0.1:8000``:

.. code-block:: console

   make run


To suggest changes online, use the GitHub link in the footer of the page
or submit a PR, limiting it to the ``docs/`` directory
and following our internal `Sphinx and Read the Docs guide
<https://canonical-documentation-with-sphinx-and-readthedocscom.readthedocs-hosted.com/>`_.


Releases
--------

#. `Snapcraft <https://snapcraft.io/docs/snapcraft>`_
   is used to build, package, and publish ``workshop`` snaps.
   All these processes run in a self-launched
   `LXD <https://documentation.ubuntu.com/lxd/en/latest/>`_ container.
   Install ``snapcraft`` and ``lxd`` using ``snap``:

   .. code-block:: console

      sudo snap install snapcraft --classic
      sudo snap install lxd


   Add the current user to the ``lxd`` group
   to give permission to access its resources:

   .. code-block:: console

      sudo usermod -a -G lxd $USER


   Log out and re-open your user session for the new group to become active,
   then initialise LXD:

   .. code-block:: console

      lxd init --minimal
