# Changelog

## [Unreleased]

### Added

- Support for "unreleasing" a release via the `-R|--unrelease` flags. Unreleasing
  works by taking the changes introduced by the last release and moving them into
  the _Unreleased_ section. If no _Unreleased_ section exists, the last release
  assumes the role.

## [0.1.0] - 2019-12-26

Initial release.

[Unreleased]: https://github.com/xuoe/kc/compare/0.1.0...HEAD
[0.1.0]: https://github.com/xuoe/kc/releases/tag/0.1.0
