# Changelog

## [0.2.2] - 2022-11-18

### Fixed

- The `roff` manual is now placed in the correct `man1` subdirectory when
  installing via `make install`.

## [0.2.1] - 2019-12-29

This is a bugfix release that ensures [building kc from source](./BUILD.md) works as intended.

## [0.2.0] - 2019-12-29

### Added

- Support for "unreleasing" a release via the `-R|--unrelease` flags. Unreleasing
  works by taking the changes introduced by the last release and moving them into
  the _Unreleased_ section. If no _Unreleased_ section exists, the last release
  assumes the role.

### Changed

- Changelog titles are now required.
- Renamed `--dump` to `--print`, which can now print nested kc properties. See
  `--print '*'` for a list of all the possible values. Each dot-separated section
  may be specified as a case-insensitive prefix and the dot characters may be
  replaced with a space character; an asterisk may be used to print the property
  names at that depth level and below.

### Fixed

- Unreleased links are now generated correctly when the _Unreleased_ section is
  the only section left after editing or removing releases.
- The documentation for the `unreleased` template was referring to an incorrect
  placeholder (`{CURRENT}`). The correct one is `{PREVIOUS}`.
- Remove unused sort flag `-S` and replace it with `-t`.
- Existing releases are now sorted prior to `--release`.

## [0.1.0] - 2019-12-26

Initial release.

[0.2.2]: https://github.com/xuoe/kc/compare/0.2.1...0.2.2
[0.2.1]: https://github.com/xuoe/kc/compare/0.2.0...0.2.1
[0.2.0]: https://github.com/xuoe/kc/compare/0.1.0...0.2.0
[0.1.0]: https://github.com/xuoe/kc/releases/tag/0.1.0
