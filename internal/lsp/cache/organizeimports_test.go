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
	"testing"

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
