# Workspace

## Installation

### Prerequisites

Workspace relies on LXD to orchestrate containers. To install and configure LXD:

```sh
sudo snap install lxd
sudo lxd init --auto
```

### Install

```
go install github.com/canonical/workspace/cmd/workspace
```

## Use

```
$ cat > .workspace.nimble.yaml <<EOF -
name: nimble
base: ubuntu@22.04
sdks:
  go:
    channel: latest/stable
  openjdk:
    channel: latest/stable
EOF
$ workspace launch
```

## Testing

### Unit tests

workspace uses a "go test"-compatible [gocheck](https://pkg.go.dev/gopkg.in/check.v1#section-readme)
```
go test ./...
go test -check.f SuiteName
```

### Functional and integrational tests

```
go install github.com/snapcore/spread/cmd/spread@latest
spread 
```
