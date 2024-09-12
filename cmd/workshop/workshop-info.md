# workshop info

Prints the current status and details of a workshop as YAML.

## Usage

```console
$ workshop info <WORKSHOP> [OPTIONS]
```

## Examples

List details for the `nimble` workshop in the current project directory:

```console
$ workshop info nimble
```

## Synopsis

This command outputs the basic settings, current status, and individual SDK
details for a workshop, formatting them as YAML. Specifically, it prints:

- Essential workshop attributes, such as name, base, and project directory.

- Current status (e.g., *Ready*, *Pending*, *Off*) and notes for the workshop.

- Individual SDK details, such as name, channel, installation date
  and revision.

- Currently mounted content interface plugs.

## Notes

- Avoid assumptions based on SDK channels: `latest/stable` may not be what you
  expect.

## Global Options

- `-h, --help`: Print the help message for the command.
- `-p, --project <DIRECTORY>`: Specify the project's directory path.

This command outputs the basic settings, current status and individual SDK
details for a workshop, formatting them as YAML. Specifically, it prints:

- Essential workshop attributes, such as name, base and project directory

- Current status (e.g. *Ready*, *Pending*, *Off*) and notes for the workshop

- Individual SDK details, such as name, channel, installation date and revision

- Currently mounted content interface plugs
