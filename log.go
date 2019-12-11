package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/pelletier/go-toml"
)

type config struct {
	path string

	// writeReleaseLinks instructs the changelog renderer to append release
	// links at the end of the changelog. No such links are written if none are
	// found in the input text, or if none can be generated from templates.
	writeReleaseLinks bool

	Links   map[string]string `toml:"links,omitempty"`
	Changes struct {
		Labels []string `toml:"labels,omitempty"`
	} `toml:"changes,omitempty"`
}

func newConfig() *config {
	return &config{
		writeReleaseLinks: true,
	}
}

func parseConfig(path string) (*config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg := newConfig()
	if err := cfg.load(f); err != nil {
		return nil, err
	}
	cfg.path = path
	return cfg, nil
}

func defaultConfig() *config {
	cfg := newConfig()
	cfg.path = "<builtin>"
	cfg.Changes.Labels = []string{
		"Added",
		"Removed",
		"Changed",
		"Security",
		"Fixed",
		"Deprecated",
	}
	return cfg
}

func (c *config) load(r io.Reader) error {
	return toml.NewDecoder(r).Decode(c)
}

func (c *config) write(w io.Writer) error {
	return toml.NewEncoder(w).
		Order(toml.OrderPreserve).
		ArraysWithOneElementPerLine(true).
		Encode(c)
}

func (a *config) merge(b *config) error {
	if a.Links == nil {
		a.Links = b.Links
	} else {
		for name, tmpl := range b.Links {
			a.Links[name] = tmpl
		}
	}
	if b.Changes.Labels != nil {
		a.Changes.Labels = b.Changes.Labels
	}
	return nil
}

func (c *config) label(label string) (string, bool) {
	for _, name := range c.Changes.Labels {
		if strings.EqualFold(name, label) {
			return name, true
		}
	}
	return label, false
}

func parseChangelog(path string, cfg *config) (*changelog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	log, err := newChangelogParser(path, cfg).parse(f)
	if err != nil {
		return nil, err
	}
	log.path = path
	return log, nil
}

type changelog struct {
	path   string
	title  string
	header string
	releases
}

func (l *changelog) release(ver string, date time.Time) (rel *release) {
	if rel = l.unreleased(); rel != nil {
		rel.version = ver
		rel.date = date
	}
	return
}

func (l *changelog) pushChange(typ string, text string) {
	unrel := l.unreleased()
	if unrel == nil {
		unrel = newUnreleased()
		l.prepend(unrel)
	}
	unrel.pushChange(typ, text)
}

func (l *changelog) validate(cfg *config) error {
	buf := new(bytes.Buffer)
	if err := l.write(buf, cfg); err != nil {
		return err
	}
	p := newChangelogParser(l.path, cfg)
	_, err := p.parse(buf)
	return err
}

func (l *changelog) save(cfg *config) error {
	return write(l.path, os.O_TRUNC|os.O_CREATE, func(f *os.File) error {
		return l.write(f, cfg)
	})
}

func (l *changelog) write(w io.Writer, cfg *config) error {
	r := newChangelogRenderer(l.path, cfg, l)
	return r.render(w)
}

type releases []*release

func (rs *releases) prepend(r *release) {
	*rs = append(releases{r}, *rs...)
}

func (rs *releases) append(r *release) {
	*rs = append(*rs, r)
}

func (rs releases) unreleased() *release {
	return rs.get("unreleased")
}

func (rs releases) get(ver string) *release {
	for _, r := range rs {
		if r.is(ver) {
			return r
		}
	}
	return nil
}

func (rs releases) has(ver string) bool {
	return rs.get(ver) != nil
}

func (rs releases) head() *release {
	return rs.at(0)
}

func (rs releases) at(i int) *release {
	if i > len(rs)-1 {
		return nil
	}
	return rs[i]
}

func (rs *releases) pop() *release {
	if rs.empty() {
		return nil
	}
	v := *rs
	r := v[0]
	v[0] = nil
	*rs = v[1:]
	return r
}

func (rs releases) empty() bool {
	return len(rs) == 0
}

func (rs releases) sort() {
	var s int
	// If the Unreleased section is at the top, it stays there. If found lower
	// on the stack, it always takes precedence over numeric version strings.
	if rs.head().unreleased() {
		s++
	}
	sort.Slice(rs[s:], func(i, j int) bool {
		var (
			a  = rs[s+i].version
			b  = rs[s+j].version
			ma = reVersion.FindStringSubmatch(a)
			mb = reVersion.FindStringSubmatch(b)
		)
		if len(ma) == 0 || len(mb) == 0 {
			// Sort lexicographically if the version strings do not resemble
			// semver, i.e., we're dealing with "unreleased".
			return a > b
		}
		for i := 0; i < reVersion.NumSubexp(); i++ {
			va, _ := strconv.Atoi(ma[i+1])
			vb, _ := strconv.Atoi(mb[i+1])
			switch {
			case va > vb:
				return true
			case va < vb:
				return false
			}

		}
		return true
	})
}

func (rs *releases) delete(vers ...string) {
	vs := *rs
	for len(vers) > 0 {
		ver := vers[0]
		vers = vers[1:]
		for i, r := range vs {
			if !r.is(ver) {
				continue
			}
			z := len(vs) - 1
			if i < z {
				copy(vs[i:], vs[i+1:])
			}
			vs[z] = nil
			vs = vs[:z]
			break
		}
	}
	*rs = vs
}

func (rs releases) stringer(fn func(*release) string) func() []string {
	return func() []string { return rs.strings(fn) }
}

func (rs releases) strings(fn func(*release) string) (res []string) {
	for _, r := range rs {
		s := fn(r)
		if s == "" {
			continue
		}
		res = append(res, s)
	}
	return
}

func (rs releases) each(fn func(*release)) {
	for _, r := range rs {
		fn(r)
	}
}

func (rs releases) match(pattern string) (res releases) {
	return rs.filter(func(r *release) bool { return r.match(pattern) })
}

func (rs releases) filter(fn func(*release) bool) (res releases) {
	for _, r := range rs {
		if fn(r) {
			res = append(res, r)
		}
	}
	return
}

type release struct {
	version string
	date    time.Time
	link    string
	note    string
	changes map[string][]string
}

var dateSeparator = strings.NewReplacer(
	"/", "-",
	".", "-",
)

const iso8601 = "2006-01-02"

func newRelease(ver, date string) *release {
	rel := &release{
		version: ver,
	}
	rel.date, _ = time.Parse(iso8601, dateSeparator.Replace(date))
	return rel
}

func newUnreleased() *release {
	return &release{version: "Unreleased"}
}

func (rel *release) unreleased() bool {
	return rel.is("unreleased")
}

func (rel *release) is(ver string) bool {
	return strings.EqualFold(rel.version, ver)
}

func (rel *release) match(pattern string) bool {
	if isGlob(pattern) {
		return rel.matchGlob(pattern)
	}
	return rel.matchPrefix(pattern)
}

func (rel *release) matchGlob(pattern string) bool {
	m, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(rel.version))
	return m
}

func (rel *release) matchPrefix(pattern string) bool {
	return strings.HasPrefix(strings.ToLower(rel.version), strings.ToLower(pattern))
}

func (rel *release) changeLabels() []string {
	return keys(rel.changes)
}

func (rel *release) changeCount() (n int) {
	for _, changes := range rel.changes {
		n += len(changes)
	}
	return
}

func (rel *release) withChangeList(typ string, do func([]string) []string) {
	if rel.changes == nil {
		rel.changes = make(map[string][]string)
	}
	rel.changes[typ] = do(rel.changes[typ])
}

func (rel *release) pushChange(typ, text string) {
	rel.withChangeList(typ, func(changes []string) []string {
		if text = strings.TrimLeftFunc(text, unicode.IsSpace); text == "" {
			return changes
		}
		return append(changes, text)
	})
}

func (rel *release) mergeChange(typ, text string) {
	rel.withChangeList(typ, func(changes []string) []string {
		if len(changes) == 0 {
			return append(changes, text)
		}
		changes[len(changes)-1] += "\n" + text
		return changes
	})
}

func (rel *release) String() string {
	if rel.unreleased() {
		return strconv.Quote(rel.version)
	}
	return rel.version
}

func (rel *release) details() string {
	n := rel.changeCount()
	return fmt.Sprintf("%s (%d %s)", rel, n, pluralize("change", n))
}

var (
	reVersion     = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)\S*?`)
	reUnreleased  = regexp.MustCompile(`(?i:^\s*\[?unreleased\]?$)`)
	reRelease     = regexp.MustCompile(`^\s*\[?(\d\.\d\.\d\S*?)\]?(?:\s+-\s+(\d{4}[-\./]\d{2}[-\./]\d{2}))?$`)
	reReleaseLink = regexp.MustCompile(`^\[([[:word:].-]+)\]:\s*(\S+)`)
)

const (
	keyUnreleased     = "unreleased"
	keyRelease        = "release"
	keyInitialRelease = "initial-release"
	keyMention        = "mention"
	keyUnlabeled      = ""
)

type changelogParser struct {
	name    string
	scanner *bufio.Scanner
	lineBuf [2]string
	lineNo  int
	rules   []*changelogPrefixParser
	log     *changelog
	config  *config
	mru     *release
}

type changelogPrefixParser struct {
	prefix string
	parse  func(string) error
}

func newChangelogParser(name string, cfg *config) *changelogParser {
	p := &changelogParser{
		name:   name,
		config: cfg,
	}
	ignore := func(string) error { return nil }
	p.rules = []*changelogPrefixParser{
		{"####", ignore},
		{"###", p.parseLabeledChanges},
		{"##", p.parseRelease},
		{"#", p.parseHeader},
		{"-", p.parseUnlabeledChanges},
		{"+", p.parseUnlabeledChanges},
	}
	return p
}

func (p *changelogParser) parse(r io.Reader) (*changelog, error) {
	p.scanner = bufio.NewScanner(r)
	p.log = new(changelog)
	var errs []error
	for p.scan() {
		line := p.line()
		for _, r := range p.rules {
			if !strings.HasPrefix(line, r.prefix) {
				continue
			}
			if err := r.parse(line); err != nil {
				errs = append(errs, p.err(err))
			}
			break
		}
	}
	if errs != nil {
		var err parseError
		err.wrap(errs...)
		return nil, err
	}
	if err := p.scanner.Err(); err != nil {
		return nil, ioError{err}
	}
	return p.log, nil
}

func (p *changelogParser) parseHeader(line string) error {
	title := strings.TrimSpace(line[1:]) // #
	if title == "" {
		return errors.New("empty changelog title")
	}
	p.log.title = title
	buf := new(strings.Builder)
LOOP:
	for p.scan() {
		line := p.line()
		switch {
		case strings.HasPrefix(line, "###"): // allow H3+
		case strings.HasPrefix(line, "[") && reReleaseLink.MatchString(line):
			if err := p.parseReleaseLink(line); err != nil {
				return err
			}
			continue
		case strings.HasPrefix(line, "#"):
			p.unscan()
			break LOOP
		}
		fmt.Fprintln(buf, line)
	}
	p.log.header = strings.TrimSpace(buf.String())
	return nil
}

func (p *changelogParser) parseRelease(line string) error {
	line = strings.TrimSpace(line[2:]) // ##
	if line == "" {
		return errors.New("empty release heading")
	}
	var rel *release
	switch {
	case reUnreleased.MatchString(line):
		rel = p.log.unreleased()
		if rel == nil {
			rel = newUnreleased()
			p.log.prepend(rel)
		}
	case reRelease.MatchString(line):
		fields := reRelease.FindStringSubmatch(line)[1:]
		ver, date := fields[0], fields[1]
		rel = p.log.get(ver)
		if rel == nil {
			rel = newRelease(ver, date)
			p.log.append(rel)
		}
	}
	if rel == nil {
		return fmt.Errorf("invalid version string: %q", line)
	}
	p.mru = rel
	return p.parseReleaseNote(rel)
}

func (p *changelogParser) parseReleaseNote(rel *release) error {
	buf := new(strings.Builder)
LOOP:
	for p.scan() {
		line := p.line()
		switch {
		case strings.HasPrefix(line, "####"): // allow H4+
		case strings.HasPrefix(line, "[") && reReleaseLink.MatchString(line):
			if err := p.parseReleaseLink(line); err != nil {
				return err
			}
			continue
		case hasAnyPrefix(line, "#-+"):
			p.unscan()
			break LOOP
		}
		fmt.Fprintln(buf, line)
	}
	str := strings.TrimSpace(buf.String())
	switch rel.note {
	case "":
		rel.note = str
	default:
		rel.note += "\n" + str
	}
	return nil
}

var errIncompatChanges = errors.New("unlabeled and labeled changes cannot coexist")

func (p *changelogParser) parseUnlabeledChanges(line string) error {
	line = strings.TrimSpace(line[1:]) // - +
	if line == "" {
		return nil // skip empty changes
	}
	rel := p.mru
	if rel == nil {
		// NOTE: this cannot happen since -/+ lists are included in the header
		// if no release heading precedes them.
		return errors.New("change is missing a version heading")
	}
	for typ := range rel.changes {
		if typ != keyUnlabeled {
			return errIncompatChanges
		}
	}
	rel.pushChange(keyUnlabeled, line)
	return p.parseChanges(rel, keyUnlabeled)
}

func (p *changelogParser) parseLabeledChanges(line string) error {
	line = strings.TrimSpace(line[3:]) // ###
	if line == "" {
		return errors.New("empty change label")
	}
	rel := p.mru
	if rel == nil {
		// NOTE: this cannot happen because ###+ headings are included in the
		// header if no release heading precedes them.
		return errors.New("change label is missing a version heading")
	}
	if rel.changes[keyUnlabeled] != nil {
		return errIncompatChanges
	}
	label, ok := p.config.label(line)
	if !ok {
		// Embed the current line number before skipping ahead.
		err := p.err(fmt.Errorf("unknown change label: %q", line))
		p.skipUntil("[#")
		return err
	}
	return p.parseChanges(rel, label)
}

func (p *changelogParser) parseChanges(rel *release, label string) error {
	for p.scan() {
		line := strings.TrimSpace(p.line())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && reReleaseLink.MatchString(line) {
			if err := p.parseReleaseLink(line); err != nil {
				return err
			}
			continue
		}
		switch line[0] {
		case '#':
			p.unscan()
			return nil
		case '*', '-':
			line = strings.TrimSpace(line[1:])
			if line == "" {
				continue
			}
			rel.pushChange(label, line)
		default:
			rel.mergeChange(label, line)
		}
	}
	return nil
}

func (p *changelogParser) parseReleaseLink(line string) error {
	// NOTE: ensure callers check whether the line matches a reReleaseLink.
	fields := reReleaseLink.FindStringSubmatch(line)[1:]
	ver, link := fields[0], fields[1]
	rel := p.log.get(ver)
	if rel == nil {
		return fmt.Errorf("release link (%s) is missing a corresponding version heading", ver)
	}
	rel.link = link
	return nil
}

func (p *changelogParser) skipUntil(chars string) {
	for p.scan() {
		if hasAnyPrefix(p.line(), chars) {
			p.unscan()
			break
		}
	}
}

func (p *changelogParser) scan() bool {
	if p.lineBuf[0] != "" {
		p.lineBuf[1], p.lineBuf[0] = p.lineBuf[0], ""
		p.lineNo++
		return true
	}
	if ok := p.scanner.Scan(); !ok {
		return false
	}
	p.lineBuf[1] = p.scanner.Text()
	p.lineNo++
	return true
}

func (p *changelogParser) unscan() {
	p.lineBuf[0], p.lineBuf[1] = p.lineBuf[1], ""
	p.lineNo--
}

func (p *changelogParser) line() string {
	return p.lineBuf[1]
}

func (p *changelogParser) err(err error) error {
	// TODO: safe to assume this is the only place that generates parseErrors?
	if _, ok := err.(parseError); ok {
		return err
	}
	if err != nil {
		// TODO: maybe find a better way to detect if we're handling a one-off
		// changelog (p.name == "") or an actual changelog file.
		var (
			msg  = "Line %d: %s"
			args = []interface{}{p.lineNo, err}
		)
		if p.name != "" {
			msg = "%s:%d: %s"
			args = append([]interface{}{p.name}, args...)
		}
		err = parseError{fmt.Errorf(msg, args...)}
	}
	return err
}

type parseError struct{ error }

func (e *parseError) wrap(errs ...error) {
	eb := new(strings.Builder)
	for i, err := range errs {
		if i > 0 {
			eb.WriteByte('\n')
		}
		eb.WriteString(err.Error())
	}
	e.error = errors.New(eb.String())
}

func (e parseError) unwrap() (errs []error) {
	if e.error == nil {
		return
	}
	for _, str := range strings.Split(e.Error(), "\n") {
		errs = append(errs, parseError{errors.New(str)})
	}
	return
}

type changelogRenderer struct {
	name   string
	log    *changelog
	config *config
	refs   []string
	*lineCounter
}

func newChangelogRenderer(name string, cfg *config, log *changelog) *changelogRenderer {
	return &changelogRenderer{
		name:   name,
		log:    log,
		config: cfg,
	}
}

func (r *changelogRenderer) render(w io.Writer) (err error) {
	r.lineCounter = &lineCounter{w: w}
	defer func() {
		switch v := recover().(type) {
		case nil:
		case renderError:
			err = fmt.Errorf("%s:%d: %s", r.name, r.lines, v.error)
		default:
			panic(v)
		}
	}()
	for _, render := range []func(io.Writer){
		r.renderHeader,
		r.renderReleases,
		r.renderReleaseLinks,
	} {
		render(r)
	}
	return
}

// renderError is used by the changelogRenderer to panic-cancel rendering.
type renderError struct{ error }

func (r *changelogRenderer) renderHeader(w io.Writer) {
	if r.log.title != "" {
		r.renderLine(w, "# %s", r.log.title)
	}
	if r.log.header != "" {
		r.renderSeparator(w)
		r.renderLine(w, r.interpolateMentions(r.log.header))
	}
}

func (r *changelogRenderer) renderReleases(w io.Writer) {
	for i, rel := range r.log.releases {
		var (
			tmpls        = r.config.Links
			heading      = rel.version
			link         = rel.link
			isUnreleased = rel.unreleased()
			isInitial    = i == len(r.log.releases)-1
			prev         = r.log.at(i + 1)
		)

		// Generate links only if a corresponding template exists;
		// otherwise, use those found in the input text, if any.
		switch {
		case isUnreleased:
			if tmpl := tmpls[keyUnreleased]; tmpl != "" && prev != nil {
				link = placeholderPrevious.interpolate(tmpl, prev.version)
			}
		case isInitial:
			if tmpl := tmpls[keyInitialRelease]; tmpl != "" {
				link = placeholderCurrent.interpolate(tmpl, rel.version)
			}
		default:
			if tmpl := tmpls[keyRelease]; tmpl != "" {
				link = placeholderPrevious.interpolate(tmpl, prev.version)
				link = placeholderCurrent.interpolate(link, rel.version)
			}
		}

		if r.config.writeReleaseLinks && link != "" {
			heading = fmt.Sprintf("[%s]", heading)
			r.refs = append(r.refs, rel.version, link)
		}
		if !rel.date.IsZero() {
			heading += " - " + rel.date.Format(iso8601)
		}

		r.renderSeparator(w)
		r.renderLine(w, "## %s", heading)
		if rel.note != "" {
			r.renderSeparator(w)
			r.renderLine(w, r.interpolateMentions(rel.note))
		}
		for _, label := range rel.changeLabels() {
			r.renderChanges(w, label, rel.changes[label])
		}
	}
}

func (r *changelogRenderer) renderChanges(w io.Writer, label string, changes []string) {
	r.renderSeparator(w)
	if label != keyUnlabeled {
		r.renderLine(w, "### %s\n", label)
	}
	for _, c := range changes {
		r.renderChange(w, c)
	}
}

func (r *changelogRenderer) renderChange(w io.Writer, change string) {
	for i, line := range strings.Split(change, "\n") {
		line = r.interpolateMentions(line)
		if i == 0 {
			r.renderLine(w, "- %s", line)
			continue
		}
		switch line[0] {
		case ' ', '\t':
		default:
			line = "  " + line
		}
		r.renderLine(w, line)
	}
}

func (r *changelogRenderer) renderReleaseLinks(w io.Writer) {
	if len(r.refs) == 0 {
		return
	}
	r.renderSeparator(w)
	for i := 0; i < len(r.refs)-1; i += 2 {
		ver, link := r.refs[i], r.refs[i+1]
		r.renderLine(w, "[%s]: %s", ver, link)
	}
}

func (r *changelogRenderer) renderLine(w io.Writer, fs string, args ...interface{}) int {
	n, err := fmt.Fprintf(w, fs+"\n", args...)
	if err != nil {
		panic(renderError{err})
	}
	return n
}

func (r *changelogRenderer) renderSeparator(w io.Writer) {
	if r.bytes > 0 && !r.hasEmptyLine {
		r.renderNewline(w)
	}
}

func (r *changelogRenderer) renderNewline(w io.Writer) int {
	return r.renderLine(w, "")
}

var reMention = regexp.MustCompile(`\[(@[[:word:]]+)\]\((.+)\)|(@[[:word:]]+)`)

func (r *changelogRenderer) interpolateMentions(str string) string {
	tmpl := r.config.Links[keyMention]
	if tmpl == "" {
		return str
	}
	return reMention.ReplaceAllStringFunc(str, func(match string) string {
		var (
			subs    = reMention.FindStringSubmatch(match)[1:]
			hasLink = subs[0] != ""
		)
		if hasLink {
			return match
		}
		mention := subs[2]
		link := placeholderMention.interpolate(tmpl, mention[1:])
		return fmt.Sprintf("[%s](%s)", mention, link)
	})
}

const (
	placeholderCurrent  = placeholder("{CURRENT}")
	placeholderPrevious = placeholder("{PREVIOUS}")
	placeholderMention  = placeholder("{MENTION}")
)

type placeholder string

func (p placeholder) interpolate(str string, val string) string {
	return strings.ReplaceAll(str, string(p), val)
}

type templates map[string]string

func (m templates) render(w io.Writer, name string, funcs template.FuncMap) error {
	tmpl, ok := m[name]
	if !ok {
		return fmt.Errorf("%s: no such template", name)
	}

	// Reduce trailing whitespace to a single newline.
	tmpl = strings.TrimRightFunc(tmpl, unicode.IsSpace)
	tmpl += "\n"

	t := template.New(name).Funcs(funcs)
	t, err := t.Parse(tmpl)
	if err != nil {
		return err
	}
	return t.Execute(w, nil)
}

// dump dumps the raw templates specified by names to w. If names is nil, all
// templates are dumped.
func (m templates) dump(w io.Writer, names ...string) error {
	b := new(strings.Builder)
	if len(names) == 0 {
		names = keys(m)
	}
	for _, name := range names {
		tmpl, ok := m[name]
		if !ok {
			// Prefer skipping unknown templates over erring out.
			continue
		}
		b.Reset()
		sep := strings.Repeat("-", len(name)+4)
		fmt.Fprintln(b, sep)
		fmt.Fprintln(b, fmt.Sprintf("| %s |", name))
		fmt.Fprintln(b, sep)
		fmt.Fprintln(b, tmpl)
		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}
	}
	return nil
}

var configTemplates = templates{
	"github": `{{ $repository := prompt "Repository" "user/repository" }}
[links]
  unreleased      = "https://github.com/{{ $repository }}/compare/{PREVIOUS}...HEAD"
  initial-release = "https://github.com/{{ $repository }}/releases/tag/{CURRENT}"
  release         = "https://github.com/{{ $repository }}/compare/{PREVIOUS}...{CURRENT}"
  mention         = "https://github.com/{MENTION}"`,

	"gitlab": `{{ $repository := prompt "Repository" "user/repository" }}
[links]
  unreleased      = "https://gitlab.com/{{ $repository }}/compare/{PREVIOUS}...master"
  initial-release = "https://gitlab.com/{{ $repository }}/-/tags/{CURRENT}"
  release         = "https://gitlab.com/{{ $repository }}/compare/{PREVIOUS}...{CURRENT}"
  mention         = "https://gitlab.com/{MENTION}"`,
}

var changelogTemplates = templates{
	"default": `# {{ prompt "Title" "Changelog" }}

## Unreleased`,
	"kacl": `# {{ prompt "Title" "Changelog" }}

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0).

## Unreleased`,
	"semver": `# {{ prompt "Title" "Changelog" }}

This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0).

## Unreleased`,
}
