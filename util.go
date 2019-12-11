package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

var reNonAlnum = regexp.MustCompile(`[^[:alnum:]]`)

func newTempPath(suffix, ext string) (string, error) {
	suffix = reNonAlnum.ReplaceAllString(suffix, "_")
	if ext != "" && ext[0] == '.' {
		ext = ext[1:]
	}
	f, err := ioutil.TempFile(os.TempDir(), fmt.Sprintf("kc_*_%s.%s", suffix, ext))
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil

}

func write(path string, mode int, fn func(*os.File) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|mode, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}

func hasAnyPrefix(s string, xyz string) bool {
	if len(s) == 0 {
		return false
	}
	for _, b := range xyz {
		if byte(b) == s[0] {
			return true
		}
	}
	return false
}

func pluralize(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func keys(m interface{}) (keys []string) {
	v := reflect.ValueOf(m)
	if v.Kind() != reflect.Map {
		panicf("keys: input type not a map")
	}
	for _, k := range v.MapKeys() {
		if k.Kind() != reflect.String {
			panicf("keys: illegal map key: %s", k.Kind())
		}
		keys = append(keys, k.Interface().(string))
	}
	sort.Strings(keys)
	return
}

func ensureMatch(vals []string, val *string) error {
	v := *val
	ms := matchPrefix(vals, v)
	if len(ms) != 1 {
		vals := strings.Join(vals, " | ")
		if v == "default" {
			return fmt.Errorf("no default value. Try one of: %s", vals)
		}
		return fmt.Errorf("%q must match at most one of: %s", v, vals)
	}
	*val = ms[0]
	return nil
}

func matchPattern(vals []string, pattern string) []string {
	if isGlob(pattern) {
		return matchGlob(vals, pattern)
	}
	return matchPrefix(vals, pattern)
}

func matchPrefix(vals []string, prefix string) (ms []string) {
	for _, val := range vals {
		if strings.HasPrefix(strings.ToLower(val), strings.ToLower(prefix)) {
			ms = append(ms, val)
		}
	}
	return
}

func matchGlob(vals []string, glob string) (ms []string) {
	for _, val := range vals {
		if ok, _ := filepath.Match(strings.ToLower(glob), strings.ToLower(val)); ok {
			ms = append(ms, val)
		}
	}
	return
}

// taken from filepath/match.go
func isGlob(path string) bool {
	magicChars := `*?[`
	if runtime.GOOS != "windows" {
		magicChars = `*?[\`
	}
	return strings.ContainsAny(path, magicChars)
}

// relativeParentDir returns a relative path to the parent of dir and whether
// that path is valid.
func relativeParentDir(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	// If the absolute path of the current directory matches that of its
	// "parent", dir already is the root directory, so abort.
	if abs == filepath.Join(abs, "..") {
		return "", false
	}
	return filepath.Join(dir, ".."), true
}
