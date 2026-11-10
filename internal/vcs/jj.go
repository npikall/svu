package vcs

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/Masterminds/semver/v3"
)

// jjBackend reads a non-colocated Jujutsu repository through the jj CLI.
// The working-copy commit (@) plays the role of git's HEAD.
type jjBackend struct {
	dir string
}

func (b jjBackend) root(ctx context.Context) (string, error) {
	out, err := jjRun(ctx, b.dir, "root")
	return strings.TrimSpace(out), err
}

func (b jjBackend) tags(ctx context.Context, tagMode string) ([]string, error) {
	revset := "tags()"
	if tagMode == TagModeCurrent {
		revset = "tags() & ::@"
	}
	out, err := jjRun(ctx, b.dir, "log", "--no-graph", "-r", revset,
		"-T", `tags.map(|t| t.name()).join("\n") ++ "\n"`)
	if err != nil {
		return nil, err
	}
	tags := strings.Fields(out)
	sortTagsNewestFirst(tags)
	return tags, nil
}

func (b jjBackend) log(ctx context.Context, tag string, dirs []string) (string, error) {
	revset := "::@"
	if tag != "" {
		revset = "tags(exact:" + strconv.Quote(tag) + ")..@"
	}
	// undescribed commits (the root, a fresh working copy) carry no
	// conventional commit message
	revset = "(" + revset + `) ~ description(exact:"")`
	args := []string{
		"log", "--no-graph", "-r", revset,
		"-T", `commit_id ++ ":" ++ description ++ ` + strconv.Quote(commitEnd),
	}
	if len(dirs) > 0 {
		args = append(args, "--")
		for _, dir := range dirs {
			args = append(args, "cwd:"+strconv.Quote(realPath(dir)))
		}
	}
	return jjRun(ctx, b.dir, args...)
}

// realPath resolves symlinks in an absolute path, as jj only accepts paths
// under the real repository root.
func realPath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		return realPath
	}
	return path
}

// sortTagsNewestFirst orders tags like git's "-version:refname" sort with
// versionsort.suffix=-: by prefix, then by semantic version, pre-releases
// before their release. Tags without a version go last.
func sortTagsNewestFirst(tags []string) {
	slices.SortStableFunc(tags, func(a, b string) int {
		return compareTags(b, a)
	})
}

func compareTags(a, b string) int {
	aPrefix, aVersion := splitTag(a)
	bPrefix, bVersion := splitTag(b)
	switch {
	case aVersion == nil && bVersion == nil:
		return strings.Compare(a, b)
	case aVersion == nil:
		return -1
	case bVersion == nil:
		return 1
	}
	return cmp.Or(strings.Compare(aPrefix, bPrefix), aVersion.Compare(bVersion))
}

// splitTag splits "v1.2.3" into "v" and 1.2.3. The version is nil when the
// tag holds none.
func splitTag(tag string) (string, *semver.Version) {
	i := strings.IndexFunc(tag, unicode.IsDigit)
	if i < 0 {
		return tag, nil
	}
	version, err := semver.NewVersion(tag[i:])
	if err != nil {
		return tag, nil
	}
	return tag[:i], version
}

func jjRun(ctx context.Context, dir string, args ...string) (string, error) {
	/* #nosec */
	cmd := exec.CommandContext(ctx, "jj", append([]string{"--color=never"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr // jj warns on stderr, keep it out of the output
	bts, err := cmd.Output()
	if err != nil {
		return "", errors.New(stderr.String())
	}
	return string(bts), nil
}
