.. _tut_sketch_sdks:

.. meta::
   :description: Tutorial on creating experimental SDKs with the 'workshop sketch-sdk'
                 command, enabling quick local SDK experiments without publishing them.

Customize with sketch SDKs
==========================

.. @tests in tests/docs-tutorial/part-3/task.yaml

This is the third section of the :ref:`four-part series <tut_index>`;
it teaches you to create experimental SDKs quickly
using the :command:`workshop sketch-sdk` command
to run local SDK experiments without publishing them.
It relies on the knowledge gained in the :ref:`tut_get_started` section,
where you learned how to create and run workshops.

Suppose you built your workshop with a number of SDKs
only to realize it still lacks some functionality you need.
Naturally, you'd like to add that,
but can you align it with the way |ws_markup| operates?

.. @artefact SDK

Fortunately, |ws_markup| allows you to quickly draft a local SDK
and use it within your workshop. This process is called *sketching*.

.. note::

   For details on how sketch SDKs are different from regular SDKs,
   see the :ref:`exp_sketch_sdk` explanation section.


Introduction
------------

We'll use the following scenario to demonstrate
how to iterate on an SDK to add missing functionality.

Suppose you're running the :samp:`dev` workshop
from the :ref:`tut_get_started` tutorial section,
additionally augmented with the :samp:`jupyter` SDK
when we discussed :ref:`tut_work_with_interfaces`:

.. code-block:: yaml
   :caption: .workshop/dev.yaml

   name: dev
   base: ubuntu@24.04
   sdks:
     - name: ollama
       channel: vulkan/stable
     - name: jupyter
     - name: system
       plugs:
         jupyter:
           interface: tunnel
           endpoint: 127.0.0.1:8989


In our example workshop,
this setup allows you to work with AI models using Ollama and Jupyter.
But what if the SDKs in your workshop don't provide some tools?
For instance, you may have a HuggingFace SDK without :program:`huggingface-cli`.
Should you create and publish an SDK just for your personal setup? Probably not.

In this guide, we'll add
`jupyter-console <https://jupyter-console.readthedocs.io/en/latest/>`_,
an interactive Python environment
that can run notebook-style code directly in the terminal.
Let's explore how to integrate this utility into your workshop
in a way that aligns with |ws_markup|.


Start sketching
---------------

Instead of manually installing tools using
:command:`workshop shell` or :command:`workshop exec`,
you can create a local SDK that automates these tasks with |ws_markup|.

.. @artefact SDK definition

Running :command:`workshop sketch-sdk`
opens a simplified version of an :ref:`SDK definition <exp_sdk_definition>`.
This defines all SDK components in a single file named :file:`sdk.yaml`:

.. @artefact workshop sketch-sdk

.. code-block:: console

   $ workshop sketch-sdk


The editor presents a minimal setup
with empty :samp:`hooks`, :samp:`plugs`, and :samp:`slots`:

.. code-block:: yaml
   :caption: sdk.yaml

   name: sketch

   hooks:
    # ...
   plugs:
    # ...
   slots:
    # ...


Under :samp:`hooks`, you'll find a commented :samp:`setup-project` section.
It runs as the :samp:`workshop` user
after the project directory is mounted and interfaces are connected.

Normally, you use it for commands that access the project files
or simply should not run as root,
so add the following to install Jupyter Console:

.. code-block:: yaml
   :caption: sdk.yaml
   :emphasize-lines: 4-6

   name: sketch

   hooks:
     setup-project: |
       source /var/lib/workshop/sdk/jupyter/venv/bin/activate
       pip install jupyter-console


This uses the existing Jupyter virtual environment,
created by the :samp:`jupyter` SDK
to install the :samp:`jupyter-console` package.

Here, the path follows a certain structure:
:file:`/var/lib/workshop/sdk/` is the location of all SDKs inside a workshop,
:file:`jupyter/` is the specific SDK name,
and :file:`venv/` is a directory specific to the SDK.

Once you save and exit :file:`sdk.yaml`,
|ws_markup| refreshes the workshop, running the new hooks:

.. code-block:: console

   ...
   Run hook "setup-project" for "sketch" SDK
   ...
   "dev" sketch refreshed


If errors occur, you can debug the sketch SDK like any other,
using :command:`workshop changes`, :command:`workshop tasks`,
and :command:`workshop refresh` with :option:`!--continue` or :option:`!--abort`.
For help, see :ref:`this guide <how_debug_issues_workshops>`.

Note that aborting the refresh does not revert your sketched changes,
so you can always restart where you left off
by running :command:`workshop sketch-sdk` again.

After the refresh,
the output of :command:`workshop info` should include something like this:

.. @artefact sketch SDK

.. code-block:: console

   $ workshop info

     ...
     sketch:
       tracking:   ~/.local/share/workshop/id/6b79e889/dev/sdk/sketch/current
       installed:  2025-08-27  (x1)


The sketch SDK entry shows the last update time and its revision (:samp:`x1`).
The SDK is local, so :samp:`tracking` lists the SDK definition path on the host;
each edit with :command:`workshop sketch-sdk` bumps the revision number.

At this point, you've created a functional, albeit simple, SDK in minutes.
Now you can use Jupyter Console to interactively work with your Ollama models.

Start the Jupyter Console
by activating the virtual environment provided by the :samp:`jupyter` SDK
and using the :program:`jupyter console` command enabled by the sketch SDK:

.. code-block:: console

   $ workshop shell
   workshop@dev:/project$ source /var/lib/workshop/sdk/jupyter/venv/bin/activate
   (jupyter-venv) workshop@dev:/project$ jupyter console

     Jupyter console 6.6.3
     ...


This opens an interactive environment where you can experiment with Ollama.
Try it out by running some Python code to interact with your models.

Install some dependencies first,
as you would normally do with Jupyter:

.. code-block:: console

   In [1]: %pip install requests


Then execute the following code to test the Ollama API:

.. code-block:: python

   import requests

   # Check if Ollama is running
   response = requests.get('http://localhost:11434/api/version')
   print(f"Ollama version: {response.json()['version']}")

   # Generate text with the tinyllama model, installed in Part 1
   data = {
       "model": "tinyllama",
       "prompt": "Why is the sky blue?",
       "stream": False
   }
   response = requests.post('http://localhost:11434/api/generate', json=data)
   print(response.json()['response'])


If everything is set up correctly,
you should see the Ollama version and a response to the prompt:

.. code-block:: console

   Ollama version: 0.9.6

   The sky is blue due to its light absorption by the air and water molecules.
   The atmosphere contains small amounts of carbon dioxide, which helps absorb
   more blue-violet light from the sun. The amount of red light absorbed also
   plays a role in determining the color of the sky. The exact combination of
   these factors can vary between different regions around the world due to
   changes in climate and topography. Additionally, some cultures have myths or
   beliefs about what colors are associated with various things, such as green
   for growth, blue for water, etc.


Quit the Jupyter console and the workshop shell
by pressing :samp:`Ctrl+D` twice.

If you need to make more changes or experiment,
just run :command:`workshop sketch-sdk` again to update your sketch SDK.
Repeat this as often as needed until it works the way you want.

.. note::

   The :command:`workshop sketch-sdk` command opens the SDK definition
   in your default text editor.
   To use a specific editor,
   set the :envvar:`EDITOR` environment variable, e.g.:

   .. code-block:: console

      $ export EDITOR=vim
      $ workshop sketch-sdk


.. note::

   For more details on SDK definition components,
   see the :ref:`explanation <exp_index>` section.
   You may want to start with :ref:`exp_sdks` and :ref:`exp_interfaces`.


Stash and restore
-----------------

You can temporarily stash the sketch SDK
to revert your workshop to its presketching state:

.. code-block:: console

   $ workshop sketch-sdk --stash
   $ workshop info


To restore the stashed SDK:

.. code-block:: console

   $ workshop sketch-sdk --restore


.. warning::

   Stashing does not delete the SDK,
   allowing you to restore it and continue working later.

   However, there's only one slot available for stashing.
   Running :command:`workshop sketch-sdk` overwrites the existing stash,
   if any.
   Be cautious to avoid losing your changes.


Explore sketches
----------------

You can only have one sketch SDK per workshop at a time;
there's no way to add :samp:`sketch-foo`, :samp:`sketch-draft`,
:samp:`sketch-final-final`, and so on.
However, a project may contain multiple workshops,
each with its own sketch SDK.

.. @artefact workshop sketches

To explore the available sketches in your project and their respective states,
use the :command:`workshop sketches` command:

.. code-block:: console

   $ workshop sketches



Convert to in-project SDK
-------------------------

.. @artefact in-project SDK
.. @artefact SDK Store

If you're happy with your sketch SDK,
your first option is to convert it into an
:ref:`in-project SDK <exp_in_project_sdk>`.
This makes it a permanent, version-controllable part of your project,
shareable with your team;
a good step before deciding to publish it to the SDK Store for wider use.

To convert the sketch, you *eject* it with the :option:`!--eject` option.
This creates a new in-project SDK
by moving the sketch's definition files
into the :file:`.workshop/` subdirectory of your project.
The original sketch SDK is removed from the workshop.
|ws_markup| can then pull the SDK directly from this directory,
bypassing the SDK Store.

By default, the new SDK is named after the project directory;
to change this, use the :option:`!--name` option:

.. code-block:: console

   $ workshop sketch-sdk --eject --name console

     "dev" sketch ejected to ".workshop/console"
     To use it, add "project-console" to the list of SDKs and run 'workshop refresh dev'


After ejecting, add the new in-project SDK to your workshop definition
(in :file:`.workshop/dev.yaml`) under the :samp:`sdks:` list,
using the :samp:`project-` prefix
so |ws_markup| knows it's an in-project SDK
and looks for it in the :file:`.workshop/` directory:

.. code-block:: yaml

   sdks:
     - name: project-console


Next, run :command:`workshop refresh` to apply the change.
If everything is set up correctly,
it's time to preserve the changes.

The definition and hooks of the newly ejected :samp:`console` SDK
are placed in the :file:`.workshop/console/` subdirectory:

.. code-block:: console

   .workshop/console/
   ├── hooks
   │   └── setup-project
   └── sdk.yaml


If your project did not previously have a :file:`.workshop/` directory,
add its contents to version control:

.. code-block:: console

   $ git add .workshop/
   $ git commit -m "Add jupyter-console in-project SDK"


This ensures your in-project SDK is tracked
and can be shared with collaborators or CI systems.

.. note::

   For a detailed comparison of in-project SDKs with other SDK types,
   see the :ref:`exp_in_project_sdk` explanation section.

   If you intend to publish a regular SDK,
   proceed to the next part of the tutorial,
   :ref:`tut_craft_sdks`.


Clean up
--------

If you're not quite satisfied with your sketching experiments,
your second option is to remove the sketch SDK permanently:

.. code-block:: console

   $ workshop sketch-sdk --remove


This deletes all changes introduced by the sketch.
Also, note that :command:`workshop remove` removes the sketch SDK,
as you could expect,
including its stashed version.


.. _tut_remove:

Remove a workshop
-----------------

When you're done with sketching,
the only thing left to cover for local workshops is the cleanup.

If you no longer need your workshop,
remove it:

.. @artefact workshop remove

.. code-block:: console

   $ workshop remove


This doesn't affect the files in the project directory,
including the workshop definition,
or any other content that was stored outside the workshop
(e.g., using the :ref:`mount interface <tut_interfaces>`
with a custom :command:`workshop remount` location;
however, the content in *default* mount locations will be deleted).

Even if you remove the workshop completely,
you can rebuild it with :command:`workshop launch`;
this may come in handy if you have removed your workshop
using the command above
before proceeding to the other parts of the tutorial.

.. warning::

   Don't delete the project directory without first removing the workshop.
   Otherwise, you'll need to manually delete the orphaned workshops;
   for help, see this how-to guide section: :ref:`how_troubleshoot_lxc`.


Next steps
----------

This was the last step in this tutorial section;
you are now familiar with the essentials of building SDKs in |ws_markup|
and have had your first taste of what sketching can achieve.

If you've mastered local SDKs,
your next step is to start creating publicly available SDKs;
proceed to the :ref:`tut_craft_sdks` section.
