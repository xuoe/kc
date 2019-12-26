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
--init [changelog]`. (See `kc --dump ch[angelog] t[emplates]` for a list of
changelog templates.)

You may also require a configuration file to alter the link generation process
or which change labels are allowed. Run `kc --init co[nfig]` and choose
a configuration template. This will create a `.kcrc` file in the current
directory. Otherwise, without a configuration file, no links are generated
beyond those already found in the changelog file.

#### Adding Changes

__kc__ operates around _changes_ and _releases_. To introduce a change, invoke
`kc [LABEL] [TEXT]` and, depending on __kc__'s configuration, you may be
prompted to enter a change label or to capture the change text in your
preferred text editor.

Prior to release, all new changes are stashed under an _Unreleased_ section.

#### Releasing Changes

To create a release, issue `kc --release`. This transforms the
_Unreleased_ section into a new release section, whose title contains an
incremented version string (as per [semver](https://semver.org/spec/v2.0.0))
and the current date in [ISO-8601
format](https://en.wikipedia.org/wiki/ISO_8601). Running `kc --list` prints
a single line: `0.0.1`.

#### Editing a Release

To edit the latest release, issue `kc --edit` without any arguments to open
the release in your preferred text editor. You can edit any of the following:
the version string, the release date, any change label and any change text.
Once done, save the file and exit the editor.

Note that if an _Unreleased_ section exists, it will be opened for editing
instead.

#### Inspecting a Release

To see the latest release and the changes introduced in the previous section,
invoke `kc --show` without any arguments.

Note that if an _Unreleased_ section exists, it will be printed instead.

#### Deleting a Release

To delete the latest release, issue `kc --delete` without any arguments. Given
the example in [Releasing Changes](#releasing-changes), running `kc --list` now
prints nothing.

Note that if changes have been added since the latest release, this deletes the
implicit _Unreleased_ section instead.

## Getting Help

`kc --help` provides a quick overview of the available commands and their
arguments. For a more thorough coverage, consult [the manual](./MANUAL.adoc)
distributed with the release package (either `MANUAL.roff` or `MANUAL.md`), or
the one generated during the [build process](./BUILD.md) (`man 1 kc`).

## License

__kc__ is released under the [MIT license](./LICENSE.md).
