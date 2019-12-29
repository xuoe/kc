[![CI Status](https://img.shields.io/github/workflow/status/xuoe/kc/CI?style=flat-square)](https://github.com/xuoe/kc/actions?query=workflow:CI)
[![Latest Release](https://img.shields.io/github/v/release/xuoe/kc?style=flat-square)](https://github.com/xuoe/kc/releases/latest)
[![Changelog](https://img.shields.io/badge/changelog-latest-blue?style=flat-square)](CHANGELOG.md)

# kc

__kc__ is a CLI utility that interfaces with changelogs that follow the [Keep
a Changelog format](https://keepachangelog.com/en/1.0.0/).

At a glance, __kc__ can

* generate changelogs,
* generate, list, edit and delete releases,
* generate release and `@mention` links,
* sort releases,
* create and group changes,
* stash unreleased changes, and
* more...

For an example changelog, see that of [Keep
a Changelog](https://github.com/olivierlacan/keep-a-changelog/blob/master/CHANGELOG.md).

## Installation

__kc__ may be installed by

- [downloading a release package](https://github.com/xuoe/kc/releases/latest)
  for your platform, unpacking it and placing the pre-compiled binary in your
  `$PATH`, or
- issuing `go get -u github.com/xuoe/kc`, or
- [building it from source](./BUILD.md), which has the benefit of also
  installing [the manual page](./MANUAL.adoc).

## Usage

This section serves as a quick guide to using `kc`. For more details, see the
[Getting Help](#getting-help) section.

#### Initialization

For most invocations, __kc__ requires a changelog file (usually `CHANGELOG.md`)
to be present either in the current directory or up the directory tree. If
a changelog does not already exist, you can initialize one by issuing `kc
--init [changelog]`. (See `kc --print changelog.templates` for a list of
changelog templates.)

You may also require a configuration file to alter the link generation process
or which change labels are allowed. Run `kc --init co[nfig]` and choose
a configuration template. This will create a `.kcrc` file in the current
directory. Otherwise, without a configuration file, no links are generated
beyond those already found in the changelog file.

#### Adding Changes

__kc__ operates around _changes_ and _releases_. To introduce a change, invoke
`kc [LABEL] [TEXT]`. If you omit `TEXT`, `kc` will open a text editor (see the
manual for [Environment](./MANUAL.adoc#Environment)) to capture the change text.
If you also omit `LABEL`, `kc` will either treat it as `TEXT` or inform you
that a label is required. (See `kc --print config.labels` for a list of change
labels.)

Prior to release, all changes are stashed under an _Unreleased_ section.

#### Releasing Changes

To release changes stashed under the _Unreleased_ section, issue `kc --release
[VERSION]`. For example, to increment the minor version number, issue `kc
--release minor`. This moves the unreleased changes into a new release section,
whose title contains the incremented version string (as per
[semver](https://semver.org/spec/v2.0.0)) and the current date in [ISO-8601
format](https://en.wikipedia.org/wiki/ISO_8601). At this point, running `kc
--list` prints a single line: `0.1.0`.

#### Editing a Release

To edit the latest release, issue `kc --edit`. This opens a text editor with
the release body ready for editing. You can edit any of the following: the
version string, the release date, any change label and any change text.
Once done, save the file and exit the editor.

Note that if the _Unreleased_ section exists, it will be opened for editing
instead.

#### Inspecting a Release

To see changes introduced by the latest release, invoke `kc --show` without any
arguments.

Note that if the _Unreleased_ section exists, it will be printed instead.

#### Deleting a Release

To delete the latest release, issue `kc --delete` without any arguments. Given
the example in [Releasing Changes](#releasing-changes), running `kc --list` now
prints nothing.

Note that if changes have been added since the latest release, this deletes the
implicit _Unreleased_ section instead.

## Getting Help

`kc --help` provides a quick overview of the available commands and their
arguments. For a more in-depth coverage, consult [the manual](./MANUAL.adoc)
distributed with the release package, or the one generated during the [build
process](./BUILD.md) (`man 1 kc`).

## License

__kc__ is released under the [MIT license](./LICENSE.md).
