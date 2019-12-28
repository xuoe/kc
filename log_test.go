package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestChangelog(t *testing.T) {
	for _, test := range []struct {
		name    string
		in, out string
		cfg     *config
		err     string
	}{
		{
			name: "no title",
			in:   "test\n",
			err:  "Line 1: missing changelog title",
		},
		{
			name: "no title",
			in:   "test## Unreleased",
			err:  "Line 1: missing changelog title",
		},
		{
			name: "no title",
			in:   "\n\n\n## Unreleased",
			err: `Line 1: missing changelog title
			Line 2: missing changelog title
			Line 3: missing changelog title`,
		},
		{
			name: "no title",
			in:   "test## Unreleased",
			err:  "Line 1: missing changelog title",
		},
		{
			name: "empty title",
			in:   "#\n",
			err:  "Line 1: missing changelog title",
		},
		{
			name: "title spacing",
			in:   "#     test\n",
			out:  "# test\n",
		},
		{
			name: "title spacing",
			in: "#			test\n",
			out: "# test\n",
		},
		{
			name: "allow no title and no header",
			in:   "## 1.0.0\n",
			out:  "## 1.0.0\n",
		},
		{
			name: "rewrite spacing",
			in: `# Changelog
			## 1.0.0
			- x
			`,
			out: `# Changelog

			## 1.0.0

			- x
			`,
		},
		{
			name: "rewrite spacing",
			in: `# Changelog
			## Unreleased
			- x

			-    y
			  z
			  abc
			## 0.1.0

			-    xyz



			`,
			out: `# Changelog

			## Unreleased

			- x
			- y
			  z
			  abc

			## 0.1.0

			- xyz
			`,
		},
		{
			name: "header",
			in: `# Changelog
			abc
			abc
			## Unreleased
			`,
			out: `# Changelog

			abc
			abc

			## Unreleased
			`,
		},
		{
			name: "header with list",
			in: `# Changelog
			header
			- a
			- b
			- c

			## Unreleased
			- x
			- y
			- z
			`,
			out: `# Changelog

			header
			- a
			- b
			- c

			## Unreleased

			- x
			- y
			- z
			`,
		},
		{
			name: "header with H3+",
			in: `# Changelog
			header
			- a
			- b
			- c

			### A
			#### B

			## Unreleased
			`,
			out: `# Changelog

			header
			- a
			- b
			- c

			### A
			#### B

			## Unreleased
			`,
		},
		{
			name: "unreleased goes first",
			in: `# Changelog
			## 2.0.0
			- x
			## 1.0.0
			- x
			## Unreleased
			- X
			`,
			out: `# Changelog

			## Unreleased

			- X

			## 2.0.0
			
			- x

			## 1.0.0
			
			- x
			`,
		},
		{
			name: "empty release",
			in: `# Changelog
			##     
			`,
			err: "Line 2: empty release heading",
		},
		{
			name: "invalid version strings",
			in: `# Changelog
			## 1
			## 2.1
			## Test
			`,
			err: `Line 2: invalid version string: "1"
			Line 3: invalid version string: "2.1"
			Line 4: invalid version string: "Test"`,
		},
		{
			name: "version strings with multiple digits",
			in: `# Changelog
			## 10.0.0
			## 123.91.0
			## 0.0.100
			`,
			out: `# Changelog

			## 10.0.0

			## 123.91.0

			## 0.0.100
			`,
		},
		{
			name: "release spacing",
			in: `# Changelog
			## Unreleased
			`,
			out: `# Changelog

			## Unreleased
			`,
		},
		{
			name: "release spacing",
			in: `# Changelog
			##			 Unreleased    
			`,
			out: `# Changelog

			## Unreleased
			`,
		},
		{
			name: "release date",
			in: `# Changelog
			## Unreleased
			## 0.1.0 - 1970-01-01
			`,
			out: `# Changelog

			## Unreleased

			## 0.1.0 - 1970-01-01
			`,
		},
		{
			name: "release date",
			in: `# Changelog
			## Unreleased
			## 0.1.0 - 1970.01.01
			`,
			out: `# Changelog

			## Unreleased

			## 0.1.0 - 1970-01-01
			`,
		},
		{
			name: "release date",
			in: `# Changelog
			## Unreleased
			## 0.1.0 - 1970/01/01
			`,
			out: `# Changelog

			## Unreleased

			## 0.1.0 - 1970-01-01
			`,
		},
		{
			name: "release note",
			in: `# Changelog
			## Unreleased

			This is a release note.
			## 0.1.0
			`,
			out: `# Changelog

			## Unreleased

			This is a release note.

			## 0.1.0
			`,
		},
		{
			name: "release note with indented list",
			in: `# Changelog
			## Unreleased

			This is a release note.
			 - x
			 - y
			 - z
			### Added
			- test
			## 0.1.0
			`,
			out: `# Changelog

			## Unreleased

			This is a release note.
			 - x
			 - y
			 - z

			### Added

			- test

			## 0.1.0
			`,
		},
		{
			name: "release links",
			in: `# Changelog
			## [Unreleased]
			## [0.1.0]
			[Unreleased]: https://example.com
			[0.1.0]: https://example.com/0.1.0
			`,
			out: `# Changelog

			## [Unreleased]

			## [0.1.0]

			[Unreleased]: https://example.com
			[0.1.0]: https://example.com/0.1.0
			`,
		},
		{
			name: "unspecified release links",
			in: `# Changelog
			## [Unreleased]
			## [0.1.0]
			`,
			out: `# Changelog

			## Unreleased

			## 0.1.0
			`,
		},
		{
			name: "invalid header link",
			in: `# Changelog
			hey
			[test x
			## Unreleased

			`,
			out: `# Changelog

			hey
			[test x

			## Unreleased
			`,
		},
		{
			name: "invalid release note link",
			in: `# Changelog
			## Unreleased

			[test
			`,
			out: `# Changelog

			## Unreleased

			[test
			`,
		},
		{
			name: "invalid change link",
			in: `# Changelog
			## Unreleased

			- change
			[test
			`,
			out: `# Changelog

			## Unreleased

			- change
			  [test
			`,
		},
		{
			name: "merge changes by version string",
			in: `# Changelog
			## 1.0.0
			- x
			## Unreleased
			- X
			## Unreleased
			- Y
			## 1.0.0
			- y
			- z
			## Unreleased
			- Z
			`,
			out: `# Changelog

			## Unreleased

			- X
			- Y
			- Z

			## 1.0.0
			
			- x
			- y
			- z
			`,
		},
		{
			name: "empty unlabeled change",
			in: `# Changelog
			## 1.0.0
			-       
			`,
			out: `# Changelog

			## 1.0.0
			`,
		},
		{
			name: "empty labeled change",
			in: `# Changelog
			## 1.0.0
			### Added
			-    
			`,
			out: `# Changelog

			## 1.0.0
			`,
		},
		{
			name: "empty change label",
			in: `# Changelog
			## 1.0.0
			###
			- x  
			`,
			err: "Line 3: empty change label",
		},
		{
			name: "unknown change label",
			in: `# Changelog
			## Unreleased
			### Nope
			- x 

			`,
			err: `Line 3: unknown change label: "Nope"`,
		},
		{
			name: "case insensitive change labels",
			in: `# Changelog
			## Unreleased
			### added
			- x 

			`,
			out: `# Changelog

			## Unreleased

			### Added

			- x
			`,
		},
		{
			name: "mix labels",
			in: `# Changelog
			## Unreleased
			- x
			### Added
			- y
			`,
			err: "Line 4: unlabeled and labeled changes cannot coexist",
		},
		{
			name: "mix labels",
			in: `# Changelog
			## Unreleased
			- x
			### added
			- y
			`,
			err: "Line 4: unlabeled and labeled changes cannot coexist",
		},
		{
			name: "merge multi-line change",
			in: `# Changelog
			## Unreleased
			- x 
			is
				a 
			   change

			`,
			out: `# Changelog

			## Unreleased

			- x
			  is
			  a
			  change
			`,
		},
		{
			name: "generate release links",
			in: `# Changelog
			## 0.1.0
			- x
			`,
			out: `# Changelog

			## [0.1.0]

			- x

			[0.1.0]: init/0.1.0
			`,
			cfg: &config{
				writeReleaseLinks: true,
				Links: map[string]string{
					"initial-release": "init/{CURRENT}",
				},
			},
		},
		{
			name: "generate release links",
			in: `# Changelog
			## Unreleased
			## 0.3.0
			- x
			## 0.2.0
			- x
			## 0.1.0
			- x
			`,
			out: `# Changelog

			## [Unreleased]

			## [0.3.0]

			- x

			## [0.2.0]

			- x

			## [0.1.0]

			- x

			[Unreleased]: unreleased/0.3.0
			[0.3.0]: rel/0.3.0
			[0.2.0]: rel/0.2.0
			[0.1.0]: init/0.1.0
			`,
			cfg: &config{
				writeReleaseLinks: true,
				Links: map[string]string{
					"initial-release": "init/{CURRENT}",
					"release":         "rel/{CURRENT}",
					"unreleased":      "unreleased/{PREVIOUS}",
				},
			},
		},
		{
			name: "generate mention links",
			in: `# Changelog

			[@xyz](external) says hi to @user.

			## Unreleased
			@user says hi back.
			`,
			out: `# Changelog

			[@xyz](external) says hi to [@user](test/user).

			## Unreleased

			[@user](test/user) says hi back.
			`,
			cfg: &config{
				Links: map[string]string{
					"mention": "test/{MENTION}",
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			check := func(err error) bool {
				if err == nil {
					return false
				}
				if test.err != "" {
					switch diff(test.err, err.Error()) {
					case "":
						return true
					default:
						t.Fatal(err)
					}
				}
				return false
			}
			cfg := test.cfg
			if cfg == nil {
				cfg = defaultConfig()
			}
			for _, s := range []*string{&test.in, &test.out, &test.err} {
				*s = noTabs(*s)
			}
			log, err := newChangelogParser("", cfg).parse(strings.NewReader(test.in))
			if check(err) {
				return
			}
			var buf bytes.Buffer
			if check(log.write(&buf, cfg)) {
				return
			}
			exp, got := test.out, buf.String()
			if diff := diff(exp, got); diff != "" {
				t.Errorf("\n%s", diff)
			}
		})
	}
}
