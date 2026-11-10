// Package vcs reads tags and commits from a version control repository.
// Git and non-colocated Jujutsu (jj) repositories are supported.
package vcs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
)

const (
	TagModeAll     = "all"
	TagModeCurrent = "current"
)

// commitEnd terminates every commit in raw log output.
const commitEnd = "<svu-commit-end>"

// Commit is a commit with a hash, title (first line of the message), and body
// (rest of the message, not including the title).
type Commit struct {
	SHA   string
	Title string
	Body  string
}

func (c Commit) String() string {
	return c.SHA + ": " + c.Title + "\n" + c.Body
}

// backend is what a version control tool must answer.
type backend interface {
	// root returns the top level directory of the repository.
	root(ctx context.Context) (string, error)
	// tags returns the tags visible in tagMode, newest version first.
	tags(ctx context.Context, tagMode string) ([]string, error)
	// log returns the commits after tag (all when tag is empty) that touch
	// dirs, newest first, each formatted as "<hash>:<message>" + commitEnd.
	log(ctx context.Context, tag string, dirs []string) (string, error)
}

// Repo is a repository svu reads versions from.
type Repo struct {
	backend backend
}

// Root returns the top level directory of the repository, or "" if unknown.
func (r Repo) Root(ctx context.Context) string {
	root, _ := r.backend.root(ctx)
	return root
}

// DescribeTag returns the newest tag matching pattern ("" matches all).
func (r Repo) DescribeTag(ctx context.Context, tagMode string, pattern string) (string, error) {
	tags, err := r.backend.tags(ctx, tagMode)
	if err != nil {
		return "", err
	}
	return selectTag(tags, pattern)
}

// Changelog returns the commits since tag that touch dirs, newest first.
func (r Repo) Changelog(ctx context.Context, tag string, dirs []string) ([]Commit, error) {
	out, err := r.backend.log(ctx, tag, dirs)
	if err != nil {
		return nil, err
	}
	return parseLog(out), nil
}

func selectTag(tags []string, pattern string) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}
	if pattern == "" {
		return tags[0], nil
	}

	g, err := glob.Compile(pattern)
	if err != nil {
		return "", err
	}
	for _, tag := range tags {
		if g.Match(tag) {
			return tag, nil
		}
	}
	return "", fmt.Errorf("no tags match '%s'", pattern)
}

func parseLog(s string) []Commit {
	var result []Commit
	for commit := range strings.SplitSeq(s, commitEnd) {
		commit = strings.TrimSpace(commit)
		if commit == "" { // accounts for the last split, which will be an empty line
			continue
		}

		hashEndIdx := strings.Index(commit, ":")
		titleEndIdx := strings.Index(commit, "\n")
		if titleEndIdx < 0 {
			titleEndIdx = len(commit)
		}

		result = append(result, Commit{
			commit[:hashEndIdx],
			commit[hashEndIdx+1 : titleEndIdx],
			commit[min(titleEndIdx+1, len(commit)):],
		})
	}
	return result
}

// Detect returns the repository that contains dir. The nearest enclosing
// ".git" or ".jj" decides; a colocated jj repository has both and is read as
// git. Without either, git is used so its errors reach the user.
func Detect(dir string) Repo {
	if isJJ(dir) {
		return Repo{jjBackend{dir}}
	}
	return Repo{gitBackend{dir}}
}

func isJJ(dir string) bool {
	d, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for {
		if exists(filepath.Join(d, ".git")) {
			return false
		}
		if exists(filepath.Join(d, ".jj")) {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
