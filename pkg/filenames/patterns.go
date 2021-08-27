package filenames

import (
	"errors"
	"path/filepath"
)

func FilterByGlobs(names, globs, notGlobs []string) (matched []string, err error) {
	if len(names) == 0 {
		return matched, errors.New("names list is empty")	
	}
	
	if len(globs) == 0 {
		return matched, errors.New("glob list is empty")
	}
	
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
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}

	return false
}

func TestPatternSlice(patterns []string) (pattern string, err error) {
	for _, pattern = range patterns {
		if _, err = filepath.Match(pattern, ""); err != nil {
			return
		}
	}

	return "", nil
}
