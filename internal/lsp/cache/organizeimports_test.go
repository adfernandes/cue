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

package cache

import (
	"bytes"
	"slices"
	"testing"

	"github.com/go-quicktest/qt"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/internal/astinternal"
)

// TestOrganizeImportsDoesNotMutateAST verifies that computing the
// organized content leaves the input AST untouched. The input is the
// file's cached parse, shared with every other consumer of the file,
// and there is no guarantee the editor ever applies the returned
// edit, so even computing-and-discarding the edit must not change
// the cached AST. The snapshot covers every node field, all
// positions (including relative positions), and attached comment
// groups.
func TestOrganizeImportsDoesNotMutateAST(t *testing.T) {
	const src = `package p

// decl doc comment
import ( // on the opening paren
	// doc for math
	"math" // trailing math

	// detached paragraph

	"mod.com/unused"
) // on the closing paren

// between declarations

// doc for strconv
import str "strconv" // trailing strconv

// after all imports

x: math.Pi
y: str.Quote("a")
`

	f, err := parser.ParseFile("test.cue", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	used := func(s *ast.ImportSpec) bool {
		return s.Path.Value != `"mod.com/unused"`
	}

	snapshot := func() []byte {
		return astinternal.AppendDebug(nil, f, astinternal.DebugConfig{AllPositions: true})
	}

	before := snapshot()
	organized := organizeImports(f, []byte(src), "\n", used)
	if organized == nil {
		t.Fatal("expected organized content")
	}
	if after := snapshot(); !bytes.Equal(before, after) {
		t.Fatalf("organizeImports mutated its input AST:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	// A second run over the unchanged AST must produce identical
	// output.
	if organized2 := organizeImports(f, []byte(src), "\n", used); !bytes.Equal(organized, organized2) {
		t.Fatalf("organizeImports is not stable over an unchanged AST:\nfirst:\n%s\nsecond:\n%s", organized, organized2)
	}
}

// organizeTestOutput parses src, organizes its imports treating the
// given quoted paths as unused, and returns the resulting content.
func organizeTestOutput(t *testing.T, src string, unused ...string) string {
	t.Helper()
	f, err := parser.ParseFile("test.cue", src, parser.ParseComments)
	qt.Assert(t, qt.IsNil(err))
	used := func(s *ast.ImportSpec) bool {
		return !slices.Contains(unused, s.Path.Value)
	}
	out := organizeImports(f, []byte(src), "\n", used)
	if out == nil {
		return src
	}
	return string(out)
}

// organizeCase is a golden organize-imports test case: src organizes
// to want, with the unused quoted paths treated as unreferenced.
type organizeCase struct {
	name string
	// The first byte of src is thrown away and is not part of the test.
	src    string
	unused []string
	// The first byte of want is thrown away and is not part of the test.
	want string
}

// TestOrganizeImportsGrouping exercises the grouping of the merged
// declaration: imports that were textually adjacent in the source
// share a group; blank lines and declaration boundaries separate
// groups; groups keep their source order and never exchange imports;
// and within every group standard-library imports (dotless first path
// segment) precede module imports, each side sorted by import path.
func TestOrganizeImportsGrouping(t *testing.T) {
	testCases := []organizeCase{
		{
			name: "split_single_mixed_run",
			src: `
package p

import (
	"mod.com/b"
	"strings"
	"mod.com/a"
)
`,
			want: `
package p

import (
	"strings"

	"mod.com/a"
	"mod.com/b"
)
`,
		},

		{
			name: "join_adjacent_single_line_decls",
			src: `
package p

import "mod.com/b"
import "mod.com/a"
`,
			want: `
package p

import (
	"mod.com/a"
	"mod.com/b"
)
`,
		},

		{
			name: "split_adjacent_single_line_decls_mixed",
			src: `
package p

import "mod.com/a"
import "strings"
`,
			want: `
package p

import (
	"strings"

	"mod.com/a"
)
`,
		},

		{
			name: "preserve_blank_between_single_line_decls",
			src: `
package p

import "mod.com/b"

import "strings"
`,
			want: `
package p

import (
	"mod.com/b"

	"strings"
)
`,
		},

		{
			name: "split_block_plus_adjacent_decl",
			src: `
package p

import (
	"strings"
	"mod.com/a"
)
import "list"
`,
			want: `
package p

import (
	"strings"

	"mod.com/a"

	"list"
)
`,
		},

		{
			name: "preserve_block_blank_decl",
			src: `
package p

import (
	"mod.com/a"
	"mod.com/b"
)

import "strings"
`,
			want: `
package p

import (
	"mod.com/a"
	"mod.com/b"

	"strings"
)
`,
		},

		{
			name: "no_fuse_adjacent_blocks",
			src: `
package p

import (
	"strings"

	"mod.com/b"
)
import (
	"list"

	"mod.com/a"
)
`,
			want: `
package p

import (
	"strings"

	"mod.com/b"

	"list"

	"mod.com/a"
)
`,
		},

		{
			name: "split_hand_made_mixed_groups",
			src: `
package p

import (
	"mod.com/b"
	"strings"

	"mod.com/a"
	"list"
)
`,
			want: `
package p

import (
	"strings"

	"mod.com/b"

	"list"

	"mod.com/a"
)
`,
		},

		{
			name:   "removal_inside_run_no_boundary",
			unused: []string{`"strings"`},
			src: `
package p

import (
	"mod.com/b"
	"strings"
	"list"
)
`,
			want: `
package p

import (
	"list"

	"mod.com/b"
)
`,
		},

		{
			name:   "removal_of_whole_group",
			unused: []string{`"list"`},
			src: `
package p

import (
	"mod.com/b"
	"strings"

	"list"
)
`,
			want: `
package p

import (
	"strings"

	"mod.com/b"
)
`,
		},

		{
			name: "all_std_single_group",
			src: `
package p

import (
	"strings"
	"math"
	"list"
)
`,
			want: `
package p

import (
	"list"
	"math"
	"strings"
)
`,
		},

		{
			name: "split_two_imports",
			src: `
package p

import (
	"mod.com/a"
	"strings"
)
`,
			want: `
package p

import (
	"strings"

	"mod.com/a"
)
`,
		},

		{
			name: "qualifier_ignored_for_classification",
			src: `
package p

import (
	"mod.com/a:q"
	"strings:s"
)
`,
			want: `
package p

import (
	"strings:s"

	"mod.com/a:q"
)
`,
		},

		{
			name: "doc_comment_joins_run",
			src: `
package p

import (
	"mod.com/a"
	// chosen codec
	"strings"
)
`,
			want: `
package p

import (
	// chosen codec
	"strings"

	"mod.com/a"
)
`,
		},

		{
			name: "trailing_comment_in_run",
			src: `
package p

import (
	"strings" // core
	"mod.com/a"
)
`,
			want: `
package p

import (
	"strings" // core

	"mod.com/a"
)
`,
		},

		{
			name:   "single_survivor_single_line_form",
			unused: []string{`"strings"`},
			src: `
package p

import (
	"mod.com/a"
	"strings"
)
`,
			want: `
package p

import "mod.com/a"
`,
		},
	}

	runOrganizeCases(t, testCases)
}

// TestOrganizeImportsComments exercises comment placement under
// grouping: a standalone comment group blank-line-separated from the
// import below it is a header of the import group that starts there
// and stays at that import group's head, while doc and trailing
// comments — and a comment group glued to the line below an import —
// travel with their import.
func TestOrganizeImportsComments(t *testing.T) {
	testCases := []organizeCase{
		{
			name: "header_stays_while_imports_sort",
			src: `
package p

import (
	"strings"

	// business schemas

	"mod.com/b"
	"mod.com/a"
)
`,
			want: `
package p

import (
	"strings"

	// business schemas

	"mod.com/a"
	"mod.com/b"
)
`,
		},

		{
			name: "header_stays_above_split_import_group",
			src: `
package p

import (
	"list"

	// section

	"mod.com/b"
	"strings"
)
`,
			want: `
package p

import (
	"list"

	// section

	"strings"

	"mod.com/b"
)
`,
		},

		{
			name:   "header_rehomes_when_import_group_removed",
			unused: []string{`"mod.com/p2"`},
			src: `
package p

import (
	"mod.com/p3"

	// section note

	"mod.com/p2"

	"mod.com/p4"
)
`,
			want: `
package p

import (
	"mod.com/p3"

	// section note

	"mod.com/p4"
)
`,
		},

		{
			name: "glued_below_previous_travels",
			src: `
package p

import (
	"mod.com/b"
	// b footnote

	"strings"
	"mod.com/a"
)
`,
			want: `
package p

import (
	"mod.com/b"
	// b footnote

	"strings"

	"mod.com/a"
)
`,
		},

		{
			name: "attached_comments_travel_across_split",
			src: `
package p

import (
	"strings"
	// why b
	"mod.com/b"
	"mod.com/a" // trailing a
)
`,
			want: `
package p

import (
	"strings"

	"mod.com/a" // trailing a
	// why b
	"mod.com/b"
)
`,
		},

		{
			name: "end_of_block_comment_group_stays",
			src: `
package p

import (
	"strings"
	"mod.com/a"

	// tail note
)
`,
			want: `
package p

import (
	"strings"

	"mod.com/a"

	// tail note
)
`,
		},

		{
			name:   "header_single_survivor_stays_detached",
			unused: []string{`"strings"`},
			src: `
package p

import (
	"strings"

	// note

	"mod.com/a"
)
`,
			want: `
package p

// note

import "mod.com/a"
`,
		},
	}

	runOrganizeCases(t, testCases)
}

func runOrganizeCases(t *testing.T, testCases []organizeCase) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := organizeTestOutput(t, tc.src[1:], tc.unused...)
			qt.Check(t, qt.Equals(got, tc.want[1:]))
		})
	}
}
