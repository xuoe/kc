package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

var (
	// These are populated at build time via -ldflags.
	buildCommit  string
	buildDate    string
	buildVersion string
)

func main() {
	inv := invocation{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	if err := inv.invoke(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}

type invocation struct {
	cmd struct {
		init      bool
		sort      bool
		print     bool
		list      bool
		listAll   bool
		show      bool
		delete    bool
		edit      bool
		release   bool
		unrelease bool
		help      bool
		version   bool
	}
	opts struct {
		config    string
		changelog string
	}
	args []string

	editor
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	cache struct {
		*changelog
		*config
	}
}

type editor func(id string, path string) ([]byte, error)

func (inv *invocation) init() error {
	if inv.stdin == nil {
		inv.stdin = os.Stdin
	}
	if inv.stdout == nil {
		inv.stdout = os.Stdout
	}
	if inv.stderr == nil {
		inv.stderr = os.Stderr
	}
	if inv.editor == nil {
		inv.editor = externalEditor(inv,
			os.Getenv("VISUAL"),
			os.Getenv("EDITOR"),
			"vim",
		)
	}
	return nil
}

func (inv *invocation) changelog() *changelog {
	if log := inv.cache.changelog; log != nil {
		return log
	}
	log, err := loadChangelog(inv.opts.changelog, inv.config())
	if err != nil {
		panic(err)
	}
	inv.cache.changelog = log
	return log
}

func (inv *invocation) config() *config {
	if cfg := inv.cache.config; cfg != nil {
		return cfg
	}
	cfg, err := loadConfig(inv.opts.config)
	if err != nil {
		panic(err)
	}
	inv.cache.config = cfg
	return cfg
}

func (inv *invocation) parse(args []string) error {
	fs := flag.NewFlagSet("kc", flag.ExitOnError)
	fs.Usage = func() { inv.doHelp() }
	fs.BoolVar(&inv.cmd.init, "init", false, "")
	fs.BoolVar(&inv.cmd.init, "i", false, "")
	fs.BoolVar(&inv.cmd.sort, "sort", false, "")
	fs.BoolVar(&inv.cmd.sort, "t", false, "")
	fs.BoolVar(&inv.cmd.print, "print", false, "")
	fs.BoolVar(&inv.cmd.print, "p", false, "")
	fs.BoolVar(&inv.cmd.list, "list", false, "")
	fs.BoolVar(&inv.cmd.list, "l", false, "")
	fs.BoolVar(&inv.cmd.listAll, "list-all", false, "")
	fs.BoolVar(&inv.cmd.listAll, "L", false, "")
	fs.BoolVar(&inv.cmd.show, "show", false, "")
	fs.BoolVar(&inv.cmd.show, "s", false, "")
	fs.BoolVar(&inv.cmd.delete, "delete", false, "")
	fs.BoolVar(&inv.cmd.delete, "d", false, "")
	fs.BoolVar(&inv.cmd.edit, "edit", false, "")
	fs.BoolVar(&inv.cmd.edit, "e", false, "")
	fs.BoolVar(&inv.cmd.release, "release", false, "")
	fs.BoolVar(&inv.cmd.release, "r", false, "")
	fs.BoolVar(&inv.cmd.unrelease, "unrelease", false, "")
	fs.BoolVar(&inv.cmd.unrelease, "R", false, "")
	fs.BoolVar(&inv.cmd.help, "help", false, "")
	fs.BoolVar(&inv.cmd.help, "h", false, "")
	fs.BoolVar(&inv.cmd.version, "version", false, "")
	fs.BoolVar(&inv.cmd.version, "v", false, "")
	fs.StringVar(&inv.opts.changelog, "changelog", "", "")
	fs.StringVar(&inv.opts.changelog, "c", "", "")
	fs.StringVar(&inv.opts.config, "config", "", "")
	fs.StringVar(&inv.opts.config, "C", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inv.args = fs.Args()
	return nil
}

const maxErrorCount = 5

func (inv *invocation) invoke(args []string) (err error) {
	defer func() {
		switch v := recover().(type) {
		case nil:
		case warning:
			err = v
		case ioError:
			err = v
		case parseError:
			err = v
		default:
			panic(v)
		}

		// NOTE: this also handles the error value returned by invoke(), not
		// just the recovered error.
		switch err := err.(type) {
		case warning:
			inv.errln(err.Error())
		case parseError:
			errs := err.unwrap()
			for i, err := range errs {
				if i == maxErrorCount {
					rest := len(errs) - i
					inv.errf("... and %d more %s\n", rest, pluralize("error", rest))
					return
				}
				inv.errln(err.Error())
			}
		case ioError:
			inv.errf("I/O Error: %s\n", err)
		case error:
			inv.errf("Error: %s\n", err)
		}
	}()
	if err := inv.init(); err != nil {
		return err
	}
	if err := inv.parse(args); err != nil {
		return err
	}
	switch {
	case inv.cmd.help:
		return inv.doHelp()
	case inv.cmd.version:
		return inv.doVersion()
	case inv.cmd.init:
		return inv.doInit()
	case inv.cmd.print:
		return inv.doPrint()
	case inv.cmd.sort:
		return inv.doSort()
	case inv.cmd.list:
		return inv.doList()
	case inv.cmd.listAll:
		return inv.doListAll()
	case inv.cmd.show:
		return inv.doShow()
	case inv.cmd.delete:
		return inv.doDelete()
	case inv.cmd.edit:
		return inv.doEdit()
	case inv.cmd.release:
		return inv.doRelease()
	case inv.cmd.unrelease:
		return inv.doUnrelease()
	default:
		return inv.doChange()
	}
}

type warning struct{ error }

func warn(s string) warning {
	return warning{errors.New(s)}
}

func warnf(fs string, args ...interface{}) warning {
	return warning{fmt.Errorf(fs, args...)}
}

var (
	warnNoChanges = warn("No changes.")
	warnNoMatches = warn("No matches.")
)

func panicf(fs string, args ...interface{}) {
	panic(fmt.Sprintf(fs, args...))
}

func (inv *invocation) doHelp() error {
	tmpl := fmt.Sprintf("kc %s\n", buildVersion) + `
Usage:
    kc [OPTIONS] <COMMAND> [ARGS]...
    kc [LABEL] [TEXT]...

Options:
    -c, --changelog <PATH>  Load the changelog found at PATH instead of auto-detecting it.
    -C, --config <PATH>     Load the config found at PATH instead of auto-detecting it.

Commands:
    -i, --init [FILE] [TEMPLATE]  Initialize a config or changelog file.
    -p, --print [PROP]...         Print or debug a property.
    -s, --show [PATTERN]          Show the "Unreleased" section or releases that match PATTERN.
    -e, --edit [PATTERN]          Like --show, but edit instead.
    -d, --delete [PATTERN]        Like --show, but delete instead.
    -l, --list [PATTERN]          List all releases or those that match PATTERN.
    -L, --list-all [PATTERN]      Like --list, but include the "Unreleased" section.
    -r, --release [VERSION]       Release the "Unreleased" section.
    -R, --unrelease               Unrelease the last release.
    -t, --sort                    Sort releases according to semver.

Arguments:
    FILE      One of "changelog" or "config"
    TEMPLATE  A template name (discoverable via --print)
    PROP      A property name (use * for a complete list)
    PATTERN   An exact version string, a version string prefix or a glob pattern
    VERSION   A version string that adheres to semver, or one of "patch", "minor", "major"

    Note that most arguments may be specified as prefixes.

Flags:
    -h, --help     Print this message.
    -v, --version  Print version information.
	`
	inv.errln(strings.TrimSpace(tmpl))
	return nil
}

func (inv *invocation) doVersion() error {
	inv.outf("%s (%s, %s)\n", buildVersion, buildCommit, buildDate)
	return nil
}

func (inv *invocation) doInit() (err error) {
	// Choose which template type to initialize.
	file := "changelog"
	if len(inv.args) >= 1 {
		file = strings.ToLower(inv.args[0])
	}
	if file, err = prefix(file).matchAs([]string{"changelog", "config"}, "file type"); err != nil {
		return err
	}
	var tmpls templates
	switch file {
	case "changelog":
		tmpls = changelogTemplates
	case "config":
		tmpls = configTemplates
	}

	// Choose which template string to initialize.
	tmpl := "default"
	if len(inv.args) >= 2 {
		tmpl = strings.ToLower(inv.args[1])
	}
	if tmpl, err = prefix(tmpl).matchAs(keys(tmpls), fmt.Sprintf("%s template", file)); err != nil {
		return err
	}

	// Write to stdout if we're not connected to a terminal.
	if !isTerminal(inv.stdout) {
		return tmpls.render(inv.stdout, tmpl, template.FuncMap{
			"prompt": inv.promptChoice,
		})
	}

	// Otherwise, attempt to write to the provided (or default) file path, but
	// don't overwrite existing files.
	var dst string
	switch file {
	case "config":
		switch {
		case inv.opts.config != "":
			dst = inv.opts.config
		case inv.opts.changelog != "":
			return errors.New("erroneous path option: --changelog. Try --config instead.")
		default:
			dst = defaultConfigName
		}
	case "changelog":
		switch {
		case inv.opts.changelog != "":
			dst = inv.opts.changelog
		case inv.opts.config != "":
			return errors.New("erroneous path option: --config. Try --changelog instead.")
		default:
			dst = defaultChangelogName
		}
	}
	if pathExists(dst) {
		return fmt.Errorf("%s: file already exists", dst)
	}
	return write(dst, os.O_CREATE|os.O_TRUNC, func(f *os.File) error {
		return tmpls.render(f, tmpl, template.FuncMap{
			"prompt": inv.promptChoice,
		})
	})
}

func (inv *invocation) doSort() error {
	log := inv.changelog()
	if len(log.releases) < 2 {
		return warn("No or too few releases to sort.")
	}
	log.sort()
	return log.save(inv.config())
}

func (inv *invocation) doPrint() (err error) {
	printers := printers{
		"changelog": printers{
			"file": printerFunc(func(inv *invocation, _ string) error {
				cfg := inv.config()
				log := inv.changelog()
				return log.write(inv.stdout, cfg)
			}),
			"path": printerFunc(func(inv *invocation, _ string) error {
				inv.outln(inv.changelog().path)
				return nil
			}),
			"templates": changelogTemplates,
			"changes": printerFunc(func(inv *invocation, _ string) error {
				var n int
				for _, rel := range inv.changelog().releases {
					n += rel.changeCount()
				}
				inv.outf("%d\n", n)
				return nil
			}),
			"releases": printerFunc(func(inv *invocation, _ string) error {
				inv.outf("%d\n", len(inv.changelog().releases))
				return nil
			}),
		},
		"config": printers{
			"file": printerFunc(func(inv *invocation, _ string) error {
				return inv.config().write(inv.stdout)
			}),
			"path": printerFunc(func(inv *invocation, _ string) error {
				inv.outln(inv.config().path)
				return nil
			}),
			"templates": configTemplates,
			"labels": printerFunc(func(inv *invocation, _ string) error {
				cfg := inv.config()
				if cfg.Changes.Labels != nil {
					for _, label := range cfg.Changes.Labels {
						inv.outln(label)
					}
				}
				return nil
			}),
		},
	}
	return printers.print(inv, strings.Join(inv.args, "."))
}

type printer interface {
	print(*invocation, string) error
}

type printerFunc func(*invocation, string) error

func (f printerFunc) print(inv *invocation, key string) error {
	return f(inv, key)
}

type printers map[string]printer

func (m printers) print(inv *invocation, key string) error {
	// NOTE: key may be one of "prop", "", "*" or "prop.*", where the dot
	// represents a parent.child relationship and the asterisk is a request
	// for printing all children starting at that depth.
	var next string
	key = strings.TrimLeft(key, ".")
	if idx := strings.Index(key, "."); idx >= 0 {
		next, key = key[idx+1:], key[:idx]
	}
	if key == "" && next != "" {
		key, next = next, ""
	}
	keys := keys(m)
	switch key {
	case "":
		for _, key := range keys {
			inv.outln(key)
		}
		return nil
	case "*":
		return m.flatten(inv, "")
	default:
		key, err := prefix(key).matchAs(keys, "key")
		if err != nil {
			return err
		}
		return m[key].print(inv, next)
	}
}

func (m printers) flatten(inv *invocation, prefix string) error {
	join := func(a, b string) string {
		// a might be prefix and prefix can be empty.
		if a == "" {
			return b
		}
		return strings.Join([]string{a, b}, ".")
	}
	for _, key := range keys(m) {
		switch next := m[key].(type) {
		case printers:
			if err := next.flatten(inv, join(prefix, key)); err != nil {
				return err
			}
		case templates:
			for _, name := range keys(next) {
				inv.outln(join(prefix, join(key, name)))
			}
		case printerFunc:
			inv.outln(join(prefix, key))
		default:
			panicf("printers.flatten: unexpected printer type: %T", next)
		}
	}
	return nil
}

// doList lists the version string for all releases (excluding the Unreleased
// section).
func (inv *invocation) doList() error {
	return inv.list(func(rel *release) string {
		if rel.unreleased() {
			return ""
		}
		return rel.String()
	})
}

// doListAll is like doList but also lists the Unreleased section prior to any
// releases and the number of changes associated with each entry.
func (inv *invocation) doListAll() error {
	return inv.list(func(rel *release) string { return rel.details() })
}

func (inv *invocation) list(sprint func(*release) string) error {
	pattern := "*"
	if len(inv.args) > 0 {
		pattern = inv.args[0]
	}
	for _, rel := range inv.changelog().match(pattern) {
		str := sprint(rel)
		if str == "" {
			continue
		}
		inv.outln(str)
	}
	return nil
}

func (inv *invocation) doShow() (err error) {
	log := inv.changelog()
	if log.empty() {
		return warn("Nothing to show.")
	}

	out := &changelog{path: log.path}
	cfg := inv.config()
	defer func() {
		if out.empty() {
			err = warnNoMatches
		} else {
			cfg.writeReleaseLinks = false
			err = out.write(inv.stdout, cfg)
		}
	}()
	var pattern string
	if len(inv.args) > 0 {
		pattern = inv.args[0]
	}
	if pattern == "" {
		out.append(log.head())
		return
	}
	for _, r := range log.match(pattern) {
		// Exclude the Unreleased section.
		if r.unreleased() {
			continue
		}
		out.append(r)
	}
	return
}

func (inv *invocation) doEdit() (err error) {
	log := inv.changelog()
	if log.empty() {
		return warn("Nothing to edit.")
	}

	edit := func(rel *release) (err error) {
		var (
			v1, v2 struct {
				*changelog
				data []byte
			}
			log = inv.changelog()
		)

		// Create a 1-release changelog (v1.changelog) and later compare it to
		// the edited changelog (v2.changelog).
		{
			v1.changelog = &changelog{
				releases: []*release{rel},
			}
			cfg := &config{
				writeReleaseLinks: false,
			}
			buf := new(bytes.Buffer)
			if err := v1.write(buf, cfg); err != nil {
				return err
			}
			v1.data = buf.Bytes()
		}

		// edit holds data related to the current edit.
		var edit = struct {
			data  []byte // actual edit data; may be reset on error to point to (v1|v2)
			path  string // path to the temporary edit file
			error        // may hold an edit error
		}{
			data: v1.data,
		}
		if path, err := newTempPath(rel.version, ".md"); err != nil {
			return err
		} else {
			edit.path = path
		}
		defer os.Remove(edit.path)
	RETRY:
		// We may jump back here if a recoverable error occurs and the user decides
		// to re-edit the data.
		if err := ioutil.WriteFile(edit.path, edit.data, 0644); err != nil {
			return err
		}

		// Edit v1 and capture changes into v2.
		if data, err := inv.editor(rel.version, edit.path); err != nil {
			return err
		} else {
			v2.data = data
			cfg := inv.config()
			r := bytes.NewReader(v2.data)
			p := newChangelogParser("", cfg)
			if log, err := p.parse(r); err != nil {
				edit.error = err
			} else {
				if err := log.validate(cfg); err != nil {
					edit.error = err
				} else {
					v2.changelog = log
				}
			}
		}

		// If no edit error occurs, determine what has changed between v1 and
		// v2 and either delete the original release or update it.
		if edit.error == nil {
			// No links are shown to the user (config.writeReleaseLinks is
			// disabled), as that would require them to keep the links in sync
			// themselves. Instead, we carry over the original link and replace
			// occurrences of the original version string with the value of the
			// modified one.
			v2.each(func(v2 *release) { v2.link = rel.link })

			switch {
			case reflect.DeepEqual(v1.changelog, v2.changelog):
				edit.error = warnNoChanges
			case len(v2.releases) > 1:
				edit.error = fmt.Errorf("Release split off into %d releases: %s", len(v2.releases), v2.releases)
			case len(v2.releases) == 1:
				mod := v2.head()
				if mod.version != rel.version && log.has(mod.version) {
					// The version header for v2 is modified and a release with that
					// version header already exists.
					edit.error = fmt.Errorf("%s is already released", mod.version)
					break
				}
				// All good, replace the original release and ensure the modified
				// release has the correct link set.
				mod.link = strings.ReplaceAll(rel.link, rel.version, mod.version)
				*rel = *mod
			case len(v2.releases) == 0:
				// Remove the release if the edit result contains no releases.
				log.delete(rel.version)
			default:
				panic("unreachable")
			}
		}

		// Finally, check if there is an error that we can recover from. If so,
		// ask the user how to proceed.
		switch edit.error {
		case nil:
		case warnNoChanges:
			err = warnNoChanges
		default:
			errstr := strings.ReplaceAll(edit.Error(), "\n", "\n  ")
			title := fmt.Sprintf("Error:\n  %s\n\nEdit again?", errstr)
			resp := inv.promptChoices(title, "y", []choice{
				{"0", "From scratch"},
				{"y", "From last edit"},
				{"n", "No"},
			})
			edit.error = nil
			switch resp {
			case "0":
				edit.data = v1.data
				goto RETRY
			case "y":
				edit.data = v2.data
				goto RETRY
			case "n":
				err = warnNoChanges
			}
		}
		return
	}

	defer func() {
		if err == nil {
			err = log.save(inv.config())
		}
	}()
	var pattern string
	if len(inv.args) > 0 {
		pattern = inv.args[0]
	}
	if pattern == "" {
		return edit(log.head())
	}
	switch res := inv.promptReleases("edit", pattern); len(res) {
	case 0:
		return warnNoMatches
	default:
		var changes int
		for i, ver := range res {
			if i > 0 {
				msg := "Continue with %s (%d/%d left)?"
				if !inv.confirmf('Y', msg, ver, len(res)-i, len(res)) {
					break
				}
			}
			switch err := edit(log.get(ver)); err {
			case nil:
				changes++
			case warnNoChanges: // ignore
			default:
				return err
			}
		}
		if changes == 0 {
			err = warnNoChanges
		}
	}
	return
}

func (inv *invocation) doDelete() (err error) {
	log := inv.changelog()
	if log.empty() {
		return warn("Nothing to delete.")
	}

	var ok bool
	confirm := func(suffix interface{}) bool {
		ok = inv.confirmf('N', "Are you sure you want to delete %s?", suffix)
		return ok
	}
	defer func() {
		if err == nil {
			switch {
			case ok:
				err = log.save(inv.config())
			case !ok:
				err = warnNoChanges
			}
		}
	}()
	var pattern string
	if len(inv.args) > 0 {
		pattern = inv.args[0]
	}
	if pattern == "" {
		if confirm(log.head().details()) {
			log.pop()
		}
		return
	}
	switch vers := inv.promptReleases("delete", pattern); len(vers) {
	case 0:
		return warnNoMatches
	case 1:
		ver := vers[0]
		if confirm(ver) {
			log.delete(ver)
		}
	default:
		if confirm(fmt.Sprintf("%d releases", len(vers))) {
			log.delete(vers...)
		}
	}
	return
}

func (inv *invocation) doRelease() error {
	log := inv.changelog()
	unrel := log.unreleased()
	if unrel == nil || (unrel.changeCount() == 0 && unrel.note == "") {
		return warn("No unreleased changes.")
	}

	// Increment the patch number by default.
	var (
		arg                    = "patch"
		do  func(string) error = inv.doReleaseBump
	)
	if len(inv.args) > 0 {
		arg = inv.args[0]
		if reVersion.MatchString(arg) {
			if log.has(arg) {
				do = inv.doReleaseMerge
			} else {
				do = inv.doReleaseVersion
			}
		}
	}
	log.sort()
	if err := do(arg); err != nil {
		return err
	}
	cfg := inv.config()
	if err := log.validate(cfg); err != nil {
		return err
	}
	return log.save(cfg)
}

func (inv *invocation) doReleaseBump(typ string) error {
	typ, err := prefix(typ).matchAs([]string{"major", "minor", "patch"}, "version number")
	if err != nil {
		return err
	}

	var (
		log = inv.changelog()
		ver = []int{0, 0, 0}
	)
	if prev := log.at(1); prev != nil {
		// Use the previous version string as a starting point.
		for i, s := range reVersion.FindStringSubmatch(prev.version)[1:] {
			ver[i], _ = strconv.Atoi(s)
		}
	}
	switch typ {
	case "major":
		ver[0]++
		ver[1] = 0
		ver[2] = 0
	case "minor":
		ver[1]++
		ver[2] = 0
	case "patch":
		ver[2]++
	}
	log.release(fmt.Sprintf("%d.%d.%d", ver[0], ver[1], ver[2]), time.Now())
	inv.outln(log.head().version)
	return nil
}

func (inv *invocation) doReleaseVersion(ver string) error {
	log := inv.changelog()
	log.release(ver, time.Now())
	inv.outln(log.head().version)
	return nil
}

func (inv *invocation) doReleaseMerge(ver string) error {
	if !inv.confirmf('N', "%s is already released. Merge unreleased changes into it?", ver) {
		return nil
	}

	var (
		log   = inv.changelog()
		cfg   = inv.config()
		rel   = log.get(ver)
		unrel = log.pop()
	)
	for label, chs := range unrel.changes {
		for _, ch := range chs {
			// NOTE: the changelog is validated by the calling function so it
			// does not matter if the label is invalid at this point.
			label, _ = cfg.label(label)
			rel.pushChange(label, ch)
		}
	}
	if then, now := rel.date.Format(iso8601), time.Now().Format(iso8601); then != now {
		if rel.date.IsZero() {
			then = "n/a"
		}
		if inv.confirmf('N', "Reset release date (%s) to the current date (%s)?", then, now) {
			rel.date = time.Now()
		}
	}
	return nil
}

func (inv *invocation) doUnrelease() error {
	log := inv.changelog()
	if log.empty() || len(log.releases) == 1 && log.head().unreleased() {
		return warn("Nothing to unrelease.")
	}
	head := log.head()
	if head.unreleased() {
		prev := log.at(1)
		prev.merge(head)
		log.delete(prev.version)
		*head = *prev
	}
	if !head.unreleased() {
		head.version = "Unreleased"
		head.date = time.Time{}
		head.link = ""
	}
	cfg := inv.config()
	if err := log.validate(cfg); err != nil {
		return err
	}
	return log.save(cfg)
}

func (inv *invocation) doChange() (err error) {
	log := inv.changelog()
	cfg := inv.config()
	var (
		label  string
		change string
		allow  = cfg.Changes.Labels
	)
	switch {
	case len(inv.args) > 0 && len(allow) == 0:
		change = strings.Join(inv.args, " ")
	case len(inv.args) > 0 && len(allow) > 0:
		label = inv.args[0]
		change = strings.Join(inv.args[1:], " ")
	}
	change = strings.TrimSpace(change)

	edit := func(change *string) error {
		path, err := newTempPath("change", ".md")
		if err != nil {
			return err
		}
		data, err := inv.editor("change", path)
		if err != nil {
			return err
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return warnNoChanges
		}
		*change = text
		return nil
	}
	push := func(label, change string) error {
		if change == "" {
			if err := edit(&change); err != nil {
				return err
			}
		}
		log.pushChange(label, change)
		return nil
	}

	defer func() {
		if err == nil {
			err = log.validate(cfg)
		}
		if err == nil {
			err = log.save(cfg)
		}
	}()
	switch {
	case label == "" && len(allow) == 0:
		return push(label, change)
	default:
		label, err := prefix(label).matchAs(allow, "change label")
		if err != nil {
			return err
		}
		return push(label, change)
	}
}

func (inv *invocation) promptReleases(act, pat string) []string {
	log := inv.changelog()
	return inv.promptList("Releases", act, pat, log.stringer(func(r *release) string {
		if r.unreleased() {
			return ""
		}
		return r.version
	}))
}

func (inv *invocation) promptList(title, act, pat string, list func() []string) []string {
	if pat == "" {
		pat = "*"
	}
	scr := newSelectionScreen(inv.stdin, inv.stderr)
	defer scr.clear()
RETRY:
	all := list()
	items := matchPattern(all, pat)
	switch len(items) {
	case 0:
		return nil
	case 1:
		return items
	}
	inv.printf(scr, "%s [%d/%d]:\n\n", title, len(items), len(all))
	for _, s := range items {
		inv.printf(scr, "  %s\n", s)
	}
	inv.printf(scr, "\n")
	pat = inv.fpromptf(scr, "Press [Return] to %s the above or apply a different pattern: ", act)
	if pat != "" {
		scr.clear()
		goto RETRY
	}
	return items
}

type choice struct {
	name string
	desc string
}

func (inv *invocation) promptChoices(title string, def string, choices []choice) string {
	scr := newSelectionScreen(inv.stdin, inv.stderr)
	defer scr.clear()
RETRY:
	inv.printf(scr, "%s\n\n", title)
	for _, c := range choices {
		inv.printf(scr, "  %s) %s\n", c.name, c.desc)
	}
	inv.printf(scr, "\n")
	choice := strings.ToLower(inv.promptChoice("Your choice", def))
	switch {
	case choice == "" && def == "":
		scr.clear()
		goto RETRY
	default:
		for _, c := range choices {
			if strings.HasPrefix(choice, c.name) {
				return choice
			}
		}
		scr.clear()
		goto RETRY
	}
}

func (inv *invocation) promptChoice(title string, def string) (res string) {
	scr := newSelectionScreen(inv.stdin, inv.stderr)
	defer scr.clear()
	if def == "" {
		return inv.fpromptf(scr, "%s: ", title)
	}
	res = inv.fpromptf(scr, "%s [%s]: ", title, def)
	if res == "" {
		res = def
	}
	return
}

func (inv *invocation) promptf(s string, args ...interface{}) string {
	return inv.prompt(fmt.Sprintf(s, args...))
}

func (inv *invocation) prompt(s string) string {
	return inv.fpromptf(inv.stderr, s)
}

func (inv *invocation) fpromptf(w io.Writer, s string, args ...interface{}) string {
	inv.printf(w, s, args...)
	return strings.TrimSpace(readLine(inv.stdin))
}

func (inv *invocation) confirmf(yn byte, fs string, args ...interface{}) bool {
	return inv.confirm(yn, fmt.Sprintf(fs, args...))
}

func (inv *invocation) confirm(yn byte, text string) bool {
	return inv.fconfirm(inv.stderr, yn, text)
}

func (inv *invocation) fconfirm(w io.Writer, yn byte, text string) bool {
	var suffix string
	switch yn {
	case 'Y', 'y':
		yn = 'y'
		suffix = "[Yn]"
	case 'N', 'n':
		yn = 'n'
		suffix = "[yN]"
	default:
		panicf(`confirm: invalid default value: %q. Must be one of "yYnN".`, yn)
	}
	// Auto-confirm if stdin is not interactive.
	if !isInteractive(inv.stdin) {
		return true
	}
RETRY:
	inv.printf(w, "%s %s ", text, suffix)
	switch inv.readByte() {
	case 'y', 'Y':
		return true
	case 'n', 'N':
		return false
	case '\n', '\r':
		return yn == 'y'
	case 0x3, 0x4, 0x1b, 0:
		// These are raw escape codes (see ascii(7)).
		//
		// 0x3:  Ctrl-c (ETX)
		// 0x4:  Ctrl-d (EOT)
		// 0x1b: ESC
		return false
	default:
		goto RETRY
	}
}

func (inv *invocation) outf(fs string, args ...interface{}) {
	inv.printf(inv.stdout, fs, args...)
}

func (inv *invocation) outln(s string) {
	inv.println(inv.stdout, s)
}

func (inv *invocation) errf(fs string, args ...interface{}) {
	inv.printf(inv.stderr, fs, args...)
}

func (inv *invocation) errln(s string) {
	inv.println(inv.stderr, s)
}

func (inv *invocation) printf(w io.Writer, fs string, args ...interface{}) {
	if _, err := fmt.Fprintf(w, fs, args...); err != nil {
		panic(ioError{err})
	}
}

func (inv *invocation) println(w io.Writer, s string) {
	if _, err := fmt.Fprintln(w, s); err != nil {
		panic(ioError{err})
	}
}

func (inv *invocation) readByte() byte {
	var (
		raw  bool
		read = readByte
	)
	if isTerminal(inv.stdin) {
		raw = true
		read = readRawByte
	}
	b := read(inv.stdin)
	if raw {
		// Always echo at least a newline when in raw mode.
		switch {
		case b >= 32:
			inv.errf("%s\n", string(b))
		default:
			inv.errf("\n")
		}
	}
	return b
}

var errFileNotFound = errors.New("file not found")

func loadConfig(userpath string) (*config, error) {
	cfg := defaultConfig()
	merge := func(path string) error {
		other, err := parseConfig(path)
		if err != nil {
			return err
		}
		cfg.path = other.path
		return cfg.merge(other)
	}

	// Attempt to merge a user-specified file with the default config.
	if userpath != "" {
		if err := merge(userpath); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	// Look for a config file in the current directory or its ancestors. If
	// found, merge it with the default config.
	other, err := findConfig(".")
	if err != nil {
		return cfg, nil
	}
	if err := merge(other); err != nil {
		return nil, err
	}
	return cfg, nil
}

const defaultConfigName = ".kcrc"

func findConfig(dir string) (string, error) {
	if path := filepath.Join(dir, defaultConfigName); pathExists(path) {
		return path, nil
	}
	up, ok := relativeParentDir(dir)
	if !ok {
		return "", errFileNotFound
	}
	return findConfig(up)
}

func loadChangelog(userpath string, cfg *config) (*changelog, error) {
	if userpath != "" {
		return parseChangelog(userpath, cfg)
	}
	path, err := findChangelog(".")
	if err != nil {
		if err == errFileNotFound {
			err = warn("No changelog found.")
		}
		return nil, err
	}
	return parseChangelog(path, cfg)
}

const defaultChangelogName = "CHANGELOG.md"

var reChangelogNames = regexp.MustCompile(fmt.Sprintf(
	`(?i:(?:%s)\.md)$`, strings.Join(changelogNames, "|"),
))

var changelogNames = []string{
	"CHANGELOG",
	"NEWS",
	"RELEASE",
	"RELEASES",
	"RELEASE-NOTES",
	"RELEASE_NOTES",
	"RELEASENOTES",
}

func findChangelog(dir string) (string, error) {
	var matches []string
	for _, dir := range []string{
		dir,
		filepath.Join(dir, "doc"),
		filepath.Join(dir, "docs"),
	} {
		info, err := ioutil.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range info {
			if f.IsDir() {
				continue
			}
			if !reChangelogNames.MatchString(f.Name()) {
				continue
			}
			matches = append(matches, filepath.Join(dir, f.Name()))
		}
	}
	switch len(matches) {
	case 0:
		up, ok := relativeParentDir(dir)
		if !ok {
			return "", errFileNotFound
		}
		return findChangelog(up)
	case 1:
		return matches[0], nil
	default:
		return "", warnf("Multiple changelogs found: %s", strings.Join(matches, ", "))
	}
}

func externalEditor(inv *invocation, exes ...string) editor {
	var editor string
	for _, p := range exes {
		if exe, err := exec.LookPath(p); err == nil {
			editor = exe
			break
		}
	}
	return func(_ string, path string) (data []byte, err error) {
		for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
			if !terminal.IsTerminal(int(f.Fd())) {
				return nil, fmt.Errorf("%s is not connected to a terminal", f.Name())
			}
		}
		if editor == "" {
			err = errors.New("No suitable editor found")
		RETRY:
			editor, err = exec.LookPath(inv.promptf("%s. Try a different executable: ", err))
			if err != nil {
				goto RETRY
			}
		}
		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		return ioutil.ReadFile(path)
	}
}
