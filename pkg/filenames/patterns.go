package filenames

import (
	"errors"

	glob "github.com/gobwas/glob"
)

func FilterByGlobs(names, globs, notGlobs []string) (matched []string, err error) {
	for _, name := range names {
		if MatchedWithAnyGlob(name, globs) && !MatchedWithAnyGlob(name, notGlobs) {
			matched = append(matched, name)
		}
	}

	if len(matched) != 0 {
		return matched, nil
	}

	for _, name := range names {
		if MatchedWithAnyGlob(name, globs) {
			return matched, errors.New(
				"all names that match at least one glob also match at least one notGlobs",
			)
		}
	}

	return matched, errors.New("no names matching glob expressions")
}

func MatchedWithAnyGlob(name string, globs []string) bool {
	for _, pattern := range globs {
		g := glob.MustCompile(pattern)

		if g.Match(name) {
			return true
		}
	}

	return false
}

func TestPatternSlice(patterns []string) (pattern string, err error) {
	for _, pattern = range patterns {
		if _, err = glob.Compile(pattern); err != nil {
			return
		}
	}

	return "", nil
}
