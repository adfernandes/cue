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

package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/internal/cuetxtar"
)

// TestCommentAttachmentImports records, as golden outputs, where the
// parser attaches comment groups in and around import declarations,
// and how the attached AST then renders through cue/format.
// Attachment here is subtle and easy to regress: doc and same-line
// comments attach to their import spec, a detached
// (blank-line-separated) group attaches backward to the preceding
// spec, and comment groups around the parentheses attach to
// positional slots on the declaration itself. The formatter and the
// LSP organize-imports code action both depend on the exact behavior,
// so a diff in these goldens is a behavior change in the parser's
// comment attachment or the formatter's comment placement.
//
// Each archive under testdata/comments holds one fixture: a
// <name>.input file followed by the goldens it produces:
//
//	out/imports/<name>.attachment: one line per comment group, with
//	the owning node and the group's Doc/Line/Position flags;
//	out/imports/<name>.roundtrip: the cue/format rendering of the
//	parsed file.
//
// CUE_UPDATE=1 regenerates the goldens.
func TestCommentAttachmentImports(t *testing.T) {
	test := cuetxtar.TxTarTest{
		Root: "testdata/comments",
		Name: "imports",
	}
	test.Run(t, func(tc *cuetxtar.Test) {
		for _, f := range tc.Archive.Files {
			base, ok := strings.CutSuffix(f.Name, ".input")
			if !ok {
				continue
			}
			pf, err := parser.ParseFile(f.Name, f.Data, parser.ParseComments)
			if err != nil {
				tc.Fatal(err)
			}

			w := tc.Writer(base + ".attachment")
			ast.Walk(pf, func(n ast.Node) bool {
				switch n.(type) {
				case *ast.Comment, *ast.CommentGroup:
					return false
				}
				for _, cg := range ast.Comments(n) {
					fmt.Fprintf(w, "%s: %q (doc=%t line=%t position=%d)\n",
						describeNode(n), strings.TrimSuffix(cg.Text(), "\n"),
						cg.Doc, cg.Line, cg.Position)
				}
				return true
			}, nil)

			out, err := format.Node(pf)
			if err != nil {
				tc.Fatal(err)
			}
			tc.Writer(base + ".roundtrip").Write(out)
		}
	})
}

// describeNode identifies the node owning a comment group, compactly.
func describeNode(n ast.Node) string {
	switch n := n.(type) {
	case *ast.File:
		return "File"
	case *ast.Package:
		return "Package"
	case *ast.ImportDecl:
		return fmt.Sprintf("ImportDecl(%d specs)", len(n.Specs))
	case *ast.ImportSpec:
		return fmt.Sprintf("ImportSpec(%s)", n.Path.Value)
	case *ast.Field:
		if id, ok := n.Label.(*ast.Ident); ok {
			return fmt.Sprintf("Field(%s)", id.Name)
		}
		return "Field(?)"
	default:
		return strings.TrimPrefix(fmt.Sprintf("%T", n), "*ast.")
	}
}
