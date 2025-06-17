.. _how_index:

How-to guides
=============

These articles
cover the needs and corner cases
that arise when you use |ws_markup| and |sdk_markup|.


Use workshops
-------------

These topics address daily |ws_markup|-related scenarios,
such as debugging individual workshops or the entire |ws_markup| installation,
moving projects within the file system, or using |ws_markup| with Git:

.. toctree::
   :hidden:

   Use workshops <use-workshops/index>

- :doc:`Add scripts to workshops <use-workshops/add-scripts>`
- :doc:`Debug issues in workshops <use-workshops/debug-workshop-issues>`
- :doc:`Fix the installation <use-workshops/troubleshoot>`
- :doc:`Forward ports <use-workshops/forward-ports>`
- :doc:`Move projects around <use-workshops/move-projects>`
- :doc:`Purge workshops <use-workshops/purge>`
- :doc:`Sketch SDKs to customize workshops <use-workshops/sketch-sdk>`
- :doc:`Use workshops with Git <use-workshops/git-workshop>`


Build SDKs
----------

.. @artefact SDK definition
.. @artefact SDK publisher
.. @artefact SDK Store

To create SDKs for |ws_markup|, SDK publishers use |sdk_markup|.
This tool is installed separately and accepts an SDK definition
to build and publish the SDK in the SDK Store (credentials provided on request):

.. toctree::
   :maxdepth: 1

   Craft SDKs <craft-sdks>


Study examples
--------------

.. @artefact SDK

This section presents a sample SDK layout for `ROS 2 <https://www.ros.org/>`_,
a popular robotics-oriented framework.
The articles discuss the design of a ROS 2-oriented SDK and its practical usage:

.. toctree::
   :hidden:

   ros2/index


- :doc:`Design an SDK <ros2/design-sdk>`
- :doc:`Create a workshop <ros2/create-workshop>`
