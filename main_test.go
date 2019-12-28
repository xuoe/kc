package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestInvoke(t *testing.T) {
	type files map[string]string
	for _, test := range []struct {
		name   string
		args   []string
		stdin  string
		edits  files  // release:text
		create files  // path:text
		expect files  // expected files
		stderr string // expected stderr
		stdout string // expected stdout
	}{
		{
			name: "simple",
			args: []string{"-p"},
			create: files{
				"CHANGELOG.md": "# Changelog\n",
			},
			expect: files{
				"CHANGELOG.md": "# Changelog\n",
			},
			stdout: "# Changelog\n",
		},
		{
			name:   "init config no template",
			args:   []string{"-i", "conf"},
			stderr: "Error: config: no default value. Try one of: github | gitlab\n",
		},
		{
			name:   "init github template",
			args:   []string{"-i", "conf", "github"},
			stdin:  "my/hub",
			stderr: "Repository [user/repository]: ",
			stdout: `
			[links]
			  unreleased      = "https://github.com/my/hub/compare/{PREVIOUS}...HEAD"
			  initial-release = "https://github.com/my/hub/releases/tag/{CURRENT}"
			  release         = "https://github.com/my/hub/compare/{PREVIOUS}...{CURRENT}"
			  mention         = "https://github.com/{MENTION}"
			`,
		},
		{
			name:   "init gitlab template",
			args:   []string{"-i", "conf", "gitlab"},
			stdin:  "my/lab",
			stderr: "Repository [user/repository]: ",
			stdout: `
			[links]
			  unreleased      = "https://gitlab.com/my/lab/compare/{PREVIOUS}...master"
			  initial-release = "https://gitlab.com/my/lab/-/tags/{CURRENT}"
			  release         = "https://gitlab.com/my/lab/compare/{PREVIOUS}...{CURRENT}"
			  mention         = "https://gitlab.com/{MENTION}"
			`,
		},
		{
			name:   "init default changelog",
			args:   []string{"-i", "ch"},
			stdin:  "Testlog",
			stderr: "Title [Changelog]: ",
			stdout: `# Testlog

			## Unreleased
			`,
		},
		{
			name:   "init semver changelog",
			args:   []string{"-i", "changel", "semver"},
			stdin:  "Semver",
			stderr: "Title [Changelog]: ",
			stdout: `# Semver

			This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0).

			## Unreleased
			`,
		},
		{
			name: "dump config",
			args: []string{"-p", "conf"},
			stdout: `
			[changes]
			  labels = [
			    "Added",
			    "Removed",
			    "Changed",
			    "Security",
			    "Fixed",
			    "Deprecated",
			  ]
			`,
		},
		{
			name: "dump builtin config path",
			args: []string{"-p", "conf", "path"},
			stdout: `<builtin>
			`,
		},
		{
			name: "dump default config path",
			create: files{
				".kcrc": `
				[changes]
				  labels = []
				`,
			},
			args: []string{"-p", "conf", "path"},
			stdout: `.kcrc
			`,
		},
		{
			name: "dump custom config path",
			create: files{
				"my-conf-file": `
				[changes]
				  labels = []
				`,
			},
			args: []string{"-C", "my-conf-file", "-p", "conf", "path"},
			stdout: `my-conf-file
			`,
		},
		{
			name: "dump changelog",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 0.1.0
				`,
			},
			args: []string{"-p", "changelog"},
			stdout: `# Changelog

			## Unreleased

			## 0.1.0
			`,
		},
		{
			name: "sort empty",
			create: files{
				"somefile.md": `# Changelog
				## 0.1.0
				## 0.3.0
				## Unreleased
                ## 1.0.0
				`,
			},
			args: []string{"-c", "somefile.md", "--sort"},
			expect: files{
				"somefile.md": `# Changelog

			## Unreleased

			## 1.0.0

			## 0.3.0

			## 0.1.0
			`,
			},
		},
		{
			name: "list",
			args: []string{"-l"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				## 1.3.0
				- Change
				## 1.1.2
				- Change
				## 2.0.0
				- Change
				`,
			},
			stdout: `1.0.0
			1.3.0
			1.1.2
			2.0.0
			`,
		},
		{
			name: "list pattern",
			args: []string{"-l", "*.1.*"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				## 1.3.0
				- Change
				## 1.1.2
				- Change
				## 2.0.0
				- Change
				`,
			},
			stdout: `1.1.2
			`,
		},
		{
			name: "list pattern",
			args: []string{"-l", "2"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				## 1.3.0
				- Change
				## 1.1.2
				- Change
				## 2.0.0
				- Change
				`,
			},
			stdout: `2.0.0
			`,
		},
		{
			name: "list no matches",
			args: []string{"-l", "2"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				`,
			},
			stderr: "",
		},
		{
			name: "list all",
			args: []string{"-L"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				## 1.3.0
				- a
				- b
				## 1.1.2
				- a
				- b
				- c
				## 2.0.0
				- Change
				`,
			},
			stdout: `"Unreleased" (0 changes)
			1.0.0 (1 change)
			1.3.0 (2 changes)
			1.1.2 (3 changes)
			2.0.0 (1 change)
			`,
		},
		{
			name: "list all pattern",
			args: []string{"-L", "unrel"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				## 1.3.0
				- a
				- b
				## 1.1.2
				- a
				- b
				- c
				## 2.0.0
				- Change
				`,
			},
			stdout: `"Unreleased" (0 changes)
			`,
		},
		{
			name: "show unreleased",
			args: []string{"-s"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				- c
				## 1.0.0
				- Change
				## 1.3.0
				- a
				- b
				## 1.1.2
				- a
				- b
				- c
				## 2.0.0
				- Change
				`,
			},
			stdout: `## Unreleased

			- a
			- b
			- c
			`,
		},
		{
			name: "show all releases",
			args: []string{"-s", "*"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				- c
				## 1.0.0
				- Change
				## 1.3.0
				- a
				- b
				## 1.1.2
				- a
				- b
				- c
				## 2.0.0
				- Change
				`,
			},
			stdin:  "\n",
			stderr: "IGNORE",
			stdout: `## 1.0.0

			- Change

			## 1.3.0

			- a
			- b

			## 1.1.2

			- a
			- b
			- c

			## 2.0.0

			- Change
			`,
		},
		{
			name: "show some releases",
			args: []string{"-s", "1.[13].?"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				- c
				## 1.0.0
				- Change
				## 1.3.0
				- a
				- b
				## 1.1.2
				- a
				- b
				- c
				## 2.0.0
				- Change
				`,
			},
			stdin:  "\n",
			stderr: "IGNORE",
			stdout: `## 1.3.0

			- a
			- b

			## 1.1.2

			- a
			- b
			- c
			`,
		},
		{
			name: "show no matches",
			args: []string{"-s", "1"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				- c
				`,
			},
			stderr: "No matches.\n",
		},
		{
			name:  "delete empty",
			args:  []string{"-d", "*"},
			stdin: "y",
			create: files{
				"CHANGELOG.md": `# Changelog
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog
				`,
			},
			stderr: "Nothing to delete.\n",
		},
		{
			name:  "delete unreleased",
			args:  []string{"-d"},
			stdin: "y",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 1.0.0

				- Change
				`,
			},
			stderr: `Are you sure you want to delete "Unreleased" (0 changes)? [yN] `,
		},
		{
			name:  "abort delete unreleased",
			args:  []string{"-d"},
			stdin: "n",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				`,
			},
			stderr: `Are you sure you want to delete "Unreleased" (0 changes)? [yN] No changes.
			`,
		},
		{
			name:  "delete all",
			args:  []string{"-d", "*"},
			stdin: "\nY",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				## 2.0.0
				- Change
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased
				`,
			},
			stderr: "IGNORE",
		},
		{
			name:  "delete selection",
			args:  []string{"-d", "1*"},
			stdin: "\nY",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				## 1.0.0
				- Change
				## 1.3.0
				- Change
				## 1.1.2
				- Change
				## 2.0.0
				- Change
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				## 2.0.0

				- Change
				`,
			},
			stderr: "IGNORE",
		},
		{
			name: "edit unreleased",
			args: []string{"-e"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a change
				- b change
				`,
			},
			edits: files{
				"Unreleased": `## unreleased
				- c change
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				- c change
				`,
			},
		},
		{
			name: "edit rename unreleased",
			args: []string{"-e"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a change
				- b change
				`,
			},
			edits: files{
				"Unreleased": `## 0.1.0
				Initial release.
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 0.1.0

				Initial release.
				`,
			},
		},
		{
			name: "edit delete unreleased",
			args: []string{"-e"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a change
				- b change
				`,
			},
			edits: files{
				"Unreleased": "",
			},
			expect: files{
				"CHANGELOG.md": "# Changelog\n",
			},
		},
		{
			name: "edit delete unreleased leave header",
			args: []string{"-e"},
			create: files{
				"CHANGELOG.md": `# Changelog
				This
				is a
				test
				## Unreleased
				- a change
				- b change
				`,
			},
			edits: files{
				"Unreleased": "",
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				This
				is a
				test
				`,
			},
		},
		{
			name:   "edit selection",
			args:   []string{"-e", "1.*"},
			stdin:  "\ny",
			stderr: "IGNORE",
			create: files{
				"CHANGELOG.md": `# Changelog
				## 1.0.0
				- 100 changes
				## 2.1.1
				- 211 changes
				## 1.1.0
				- 110 changes
				## 2.1.0
				- 210 changes
				## Unreleased
				- a change
				- b change
				`,
			},
			edits: files{
				"1.0.0": `## 1.0.0
				- 100 CHANGES
				`,
				"1.1.0": `## 1.1.0
				- 110 CHANGES
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				- a change
				- b change

				## 1.0.0

				- 100 CHANGES

				## 2.1.1

				- 211 changes

				## 1.1.0

				- 110 CHANGES

				## 2.1.0

				- 210 changes
				`,
			},
		},
		{
			name:   "edit already released",
			args:   []string{"-e", "1.0.0"},
			stdin:  "n",
			stderr: "IGNORE",
			create: files{
				"CHANGELOG.md": `# Changelog
				## 1.0.0
				- a change
				- b change

				## 2.0.0
				- a change
				- b change
				`,
			},
			edits: files{
				"1.0.0": `## 2.0.0`,
			},
		},
		{
			name:   "release no args",
			args:   []string{"-r"},
			stdout: "0.0.1\n",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 0.0.1 - {TEST_DATE}

				- a
				- b
				`,
			},
		},
		{
			name:   "release patch",
			args:   []string{"-r", "pa"},
			stdout: "0.0.1\n",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 0.0.1 - {TEST_DATE}

				- a
				- b
				`,
			},
		},
		{
			name:   "release minor",
			args:   []string{"-r", "mi"},
			stdout: "0.1.0\n",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 0.1.0 - {TEST_DATE}

				- a
				- b
				`,
			},
		},
		{
			name:   "release major",
			args:   []string{"-r", "maj"},
			stdout: "1.0.0\n",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 1.0.0 - {TEST_DATE}

				- a
				- b
				`,
			},
		},
		{
			name:   "release version",
			args:   []string{"-r", "5.0.0"},
			stdout: "5.0.0\n",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 5.0.0 - {TEST_DATE}

				- a
				- b
				`,
			},
		},
		{
			name:   "release merge versions",
			args:   []string{"-r", "0.1.0"},
			stdin:  "yy",
			stderr: "IGNORE",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b

				## 0.1.0
				- c 
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## 0.1.0 - {TEST_DATE}

				- c
				- a
				- b
				`,
			},
		},
		{
			name: "unrelease previous release",
			args: []string{"-R"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				- a
				- b

				## 0.1.0
				- c
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				- c
				- a
				- b
				`,
			},
		},
		{
			name: "unrelease only release",
			args: []string{"-R"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## 0.1.0
				- a
				b
				- c

				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				- a
				  b
				- c
				`,
			},
		},
		{
			name: "unrelease merge release notes",
			args: []string{"-R"},
			create: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				Second note

				- some change

				## 0.1.0

				First note

				- a
				b
				- c

				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				First note

				Second note

				- a
				  b
				- c
				- some change
				`,
			},
		},
		{
			name:   "unrelease empty log",
			args:   []string{"-R"},
			stderr: "Nothing to unrelease.\n",
			create: files{
				"CHANGELOG.md": `# Changelog
				`,
			},
		},
		{
			name:   "unrelease unreleased-only log",
			args:   []string{"-R"},
			stderr: "Nothing to unrelease.\n",
			create: files{
				"CHANGELOG.md": `# Changelog
				## Unreleased
				`,
			},
		},
		{
			name:   "unrelease and validate",
			args:   []string{"-R"},
			stderr: "CHANGELOG.md:8: unlabeled and labeled changes cannot coexist\n",
			create: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				### Added

				- test change

				## 0.1.0

				- a
				- b
				`,
			},
		},
		{
			name: "change label prefix",
			args: []string{"a"},
			create: files{
				"CHANGELOG.md": `# Changelog`,
			},
			edits: files{
				"change": "test!",
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				### Added

				- test!
				`,
			},
		},
		{
			name: "change inline",
			args: []string{"a", "this is a change"},
			create: files{
				"CHANGELOG.md": `# Changelog`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				### Added

				- this is a change
				`,
			},
		},
		{
			name: "change after release",
			args: []string{"a", "test change"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## 0.1.0

				### Added
				- old added change

				### Removed
				- old remove change
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				### Added

				- test change

				## 0.1.0

				### Added

				- old added change

				### Removed

				- old remove change
				`,
			},
		},
		{
			name: "change after note-only release",
			args: []string{"a", "test change"},
			create: files{
				"CHANGELOG.md": `# Changelog
				## 0.1.0

				This is a note.
				`,
			},
			expect: files{
				"CHANGELOG.md": `# Changelog

				## Unreleased

				### Added

				- test change

				## 0.1.0

				This is a note.
				`,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Create a temporary directory and cd into it.
			dir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			defer cd(t, cd(t, dir))

			// Populate the directory with whatever test files we need.
			for name, text := range test.create {
				if err := ioutil.WriteFile(name, []byte(noTabs(text)), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Create the invocation.
			var (
				stderr = new(bytes.Buffer)
				stdout = new(bytes.Buffer)
			)
			inv := invocation{
				stdin:  bytes.NewBufferString(test.stdin),
				stderr: stderr,
				stdout: stdout,
				editor: func(name string, path string) ([]byte, error) {
					if test.edits == nil {
						t.Fatal("unexpected edit", name, path)
					}
					text, ok := test.edits[name]
					if ok {
						return []byte(noTabs(text)), nil
					}
					return nil, errors.New("missing edit data")
				},
			}

			// Invoke, but ignore the error, which is already printed to
			// stderr.
			inv.invoke(test.args)

			// Group actual and expected outputs/files.
			exp := map[string]string{
				"stdout": noTabs(test.stdout),
				"stderr": noTabs(test.stderr),
			}
			now := time.Now().Format(iso8601)
			for name, text := range test.expect {
				text = strings.ReplaceAll(text, "{TEST_DATE}", now)
				text = noTabs(text)
				exp[name] = text
			}
			got := map[string]string{
				"stdout": stdout.String(),
				"stderr": stderr.String(),
			}
			for name := range test.expect {
				data, err := ioutil.ReadFile(name)
				if err != nil {
					t.Log(name, err)
					continue
				}
				got[name] = string(data)
			}

			// Compare.
			for name, data := range exp {
				exp, got := data, got[name]
				if exp == "IGNORE" {
					continue
				}
				if diff := diff(exp, got); diff != "" {
					t.Errorf("\n%s:\n%s", name, diff)
				}
			}
		})
	}
}

func cwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}

func cd(t *testing.T, dir string) string {
	t.Helper()
	cwd := cwd(t)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return cwd
}

var (
	reEOL        = regexp.MustCompile(`(\r\n|\r|\n)`)
	reSpace      = regexp.MustCompile(`(?m:^[\t ]+|[\t +]$)`)
	replaceSpace = func(m string) string {
		m = strings.Replace(m, " ", "·", -1)
		m = strings.Replace(m, "\t", "~", -1)
		return m
	}
)

func diff(a, b string) string {
	if a == b {
		return ""
	}
	var (
		buf   strings.Builder
		dmp   = diffmatchpatch.New()
		diffs = dmp.DiffMain(a, b, true)
	)
	for _, diff := range diffs {
		text := diff.Text
		text = reSpace.ReplaceAllStringFunc(text, replaceSpace)
		text = reEOL.ReplaceAllString(text, "$\n")
		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			buf.WriteString("\x1b[32m")
			buf.WriteString(text)
			buf.WriteString("\x1b[0m")
		case diffmatchpatch.DiffDelete:
			buf.WriteString("\x1b[31m")
			buf.WriteString(text)
			buf.WriteString("\x1b[0m")
		case diffmatchpatch.DiffEqual:
			buf.WriteString(text)
		}
	}
	return buf.String()
}

var reLeadTabs = regexp.MustCompile("(?m:^\t+)")

func noTabs(s string) string {
	return reLeadTabs.ReplaceAllLiteralString(s, "")
}
