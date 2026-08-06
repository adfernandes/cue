// Copyright 2026 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pkg

import (
	"embed"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
)

// Each standard library package has a generated pkg.cue file
// alongside its generated pkg.go file, declaring each of the
// package's members. Functions are declared as required fields — the
// definition file cannot carry their values — whose @stdlib attribute
// records the signature, such as
//
//	Compare!: _ @stdlib(func(_~a: string, _~b: string) -> int)
//
// where each parameter is an anonymous positional carrying its Go
// name as a non-contract alias. A builtin intended for use as a
// validator, which its Go signature declares by returning
// [cuelang.org/go/internal/pkg.Validator], uses the validator form,
// func(_~min: int) -> validator(string), or validator(bytes|string)
// when the validated value is its only parameter. Constants are
// declared with their concrete values, and members that are defined
// in CUE appear as their original declarations. Doc comments are
// carried over from the corresponding Go declarations.
//
// These files describe the standard library; they are not its
// implementation and are not loaded during evaluation. They exist for
// tooling that needs to present the standard library without
// evaluating it, such as editor completion and hover documentation.
//
// The go:embed patterns match only pkg.cue files, so that stray files
// cannot end up embedded in the binary. Patterns cannot recurse (**
// is not supported), so each directory depth is named;
// TestEmbedComplete fails if a pkg.cue file on disk is not covered.
//
//go:embed */pkg.cue */*/pkg.cue
var defFS embed.FS

// Source returns the CUE source of the generated definitions
// describing the API of the standard library package with the given
// import path, such as "strings" or "encoding/json". It reports false
// if there is no such package.
//
// Experimental: this API may change.
func Source(importPath string) ([]byte, bool) {
	b, err := defFS.ReadFile(path.Join(importPath, "pkg.cue"))
	if err != nil {
		return nil, false
	}
	return b, true
}

// ImportPaths returns the import paths of all standard library
// packages, sorted lexically.
//
// Experimental: this API may change.
func ImportPaths() []string {
	return importPaths()
}

var importPaths = sync.OnceValue(func() []string {
	var paths []string
	err := fs.WalkDir(defFS, ".", func(filename string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Base(filename) != "pkg.cue" {
			return err
		}
		paths = append(paths, strings.TrimSuffix(filename, "/pkg.cue"))
		return nil
	})
	if err != nil {
		// The embedded file system is part of the build; walking it
		// cannot fail.
		panic(err)
	}
	// fs.WalkDir visits entries in lexical order per directory, but a
	// path such as math/bits sorts differently from math relative to
	// the other top-level entries, so the trimmed paths are not
	// necessarily sorted.
	slices.Sort(paths)
	return paths
})
