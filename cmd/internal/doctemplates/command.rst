.. _{{ .Ref }}:


.. meta::
   :description: Reference documentation for the '{{ .CommandName }}' command

{{ .CommandName }}
{{ repeat "-" .HeadingLen }}

.. @artefact {{ .CommandName }}

{{ .Short | trimSuffix "." }}.

.. rubric:: Usage

.. code-block:: console

   $ {{ .Synopsis }}

.. rubric:: Description

{{ .Long | trimSpace }}

{{- if .Examples }}

.. rubric:: Examples
{{ range .Examples }}
{{ .Info }}

.. code-block:: console

{{ .Usage | indent 3 }}
{{ end }}
{{- end }}

{{- if .Flags }}

.. rubric:: Flags
{{ range .Flags }}
{{ if .Shorthand }}-{{ .Shorthand }}, {{ end }}--{{ .Name }}

{{ .Usage | indent 3 }}
{{- if and .DefaultValue (ne .Type "bool") (ne .DefaultValue "[]") }}

   Default: ``{{ .DefaultValue }}``
{{- end }}
{{ end }}
{{- end }}

{{- if .RelatedCommands }}

.. rubric:: See also

Reference:
{{ range .RelatedCommands }}
- :ref:`ref_{{ . | replaceSpaces }}`
{{- end }}
{{- end }}
