// Copyright 2020 CUE Authors
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

package astutil_test

import (
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/internal"

	"github.com/go-quicktest/qt"
)

func TestSanitize(t *testing.T) {
	testCases := []struct {
		desc string
		file *ast.File
		want string
	}{{
		desc: "Take existing import and rename it",
		file: func() *ast.File {
			spec := ast.NewImport(nil, "list")
			spec.AddComment(internal.NewComment(true, "will be renamed"))
			return &ast.File{Decls: []ast.Decl{
				&ast.ImportDecl{Specs: []*ast.ImportSpec{spec}},
				&ast.EmbedDecl{
					Expr: ast.NewStruct(
						ast.NewIdent("list"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "list", Node: spec},
								"Min")),
					)},
			}}
		}(),
		want: `import (
	// will be renamed
	list_9 "list"
)

{
	list: list_9.Min()
}
`,
	}, {
		desc: "Take existing import and rename it",
		file: func() *ast.File {
			spec := ast.NewImport(nil, "list")
			return &ast.File{Decls: []ast.Decl{
				&ast.ImportDecl{Specs: []*ast.ImportSpec{spec}},
				&ast.Field{
					Label: ast.NewIdent("a"),
					Value: ast.NewStruct(
						ast.NewIdent("list"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "list", Node: spec}, "Min")),
					),
				},
			}}
		}(),
		want: `import list_9 "list"

a: {
	list: list_9.Min()
}
`,
	}, {
		desc: "One import added, one removed",
		file: &ast.File{Decls: []ast.Decl{
			&ast.ImportDecl{Specs: []*ast.ImportSpec{
				{Path: ast.NewString("foo")},
			}},
			&ast.Field{
				Label: ast.NewIdent("a"),
				Value: ast.NewCall(
					ast.NewSel(&ast.Ident{
						Name: "bar",
						Node: &ast.ImportSpec{Path: ast.NewString("bar")},
					}, "Min")),
			},
		}},
		want: `import "bar"

a: bar.Min()
`,
	}, {
		desc: "Rename duplicate import",
		file: func() *ast.File {
			spec1 := ast.NewImport(nil, "bar")
			spec2 := ast.NewImport(nil, "foo/bar")
			spec3 := ast.NewImport(ast.NewIdent("bar"), "foo")
			return &ast.File{Decls: []ast.Decl{
				&ast.CommentGroup{List: []*ast.Comment{{Text: "// File comment"}}},
				&ast.Package{Name: ast.NewIdent("pkg")},
				&ast.Field{
					Label: ast.NewIdent("a"),
					Value: ast.NewStruct(
						ast.NewIdent("b"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "bar", Node: spec1}, "A")),
						ast.NewIdent("c"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "bar", Node: spec2}, "A")),
						ast.NewIdent("d"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "bar", Node: spec3}, "A")),
					),
				},
			}}
		}(),
		want: `// File comment

package pkg

import (
	"bar"
	bar_9 "foo/bar"
	bar_B "foo"
)

a: {
	b: bar.A()
	c: bar_9.A()
	d: bar_B.A()
}
`,
	}, {
		desc: "Rename duplicate import, reuse and drop",
		file: func() *ast.File {
			spec1 := ast.NewImport(nil, "bar")
			spec2 := ast.NewImport(nil, "foo/bar")
			spec3 := ast.NewImport(ast.NewIdent("bar"), "foo")
			return &ast.File{Decls: []ast.Decl{
				&ast.ImportDecl{Specs: []*ast.ImportSpec{
					spec3,
					ast.NewImport(nil, "foo"),
				}},
				&ast.Field{
					Label: ast.NewIdent("a"),
					Value: ast.NewStruct(
						ast.NewIdent("b"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "bar", Node: spec1}, "A")),
						ast.NewIdent("c"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "bar", Node: spec2}, "A")),
						ast.NewIdent("d"), ast.NewCall(
							ast.NewSel(&ast.Ident{Name: "bar", Node: spec3}, "A")),
					),
				},
			}}
		}(),
		want: `import (
	bar "foo"
	bar_9 "bar"
	bar_B "foo/bar"
)

a: {
	b: bar_9.A()
	c: bar_B.A()
	d: bar.A()
}
`,
	}, {
		desc: "Reuse different import",
		file: &ast.File{Decls: []ast.Decl{
			&ast.Package{Name: ast.NewIdent("pkg")},
			&ast.ImportDecl{Specs: []*ast.ImportSpec{
				{Path: ast.NewString("bar")},
			}},
			&ast.Field{
				Label: ast.NewIdent("a"),
				Value: ast.NewStruct(
					ast.NewIdent("list"), ast.NewCall(
						ast.NewSel(&ast.Ident{
							Name: "bar",
							Node: &ast.ImportSpec{Path: ast.NewString("bar")},
						}, "Min")),
				),
			},
		}},
		want: `package pkg

import "bar"

a: {
	list: bar.Min()
}
`,
	}, {
		desc: "Clear reference that does not exist in scope",
		file: &ast.File{Decls: []ast.Decl{
			&ast.Field{
				Label: ast.NewIdent("a"),
				Value: ast.NewStruct(
					ast.NewIdent("b"), &ast.Ident{
						Name: "c",
						Node: ast.NewString("foo"),
					},
					ast.NewIdent("d"), ast.NewIdent("e"),
				),
			},
		}},
		want: `a: {
	b: c
	d: e
}
`,
	}, {
		desc: "Unshadow possible reference to other file",
		file: &ast.File{Decls: []ast.Decl{
			&ast.Field{
				Label: ast.NewIdent("a"),
				Value: ast.NewStruct(
					ast.NewIdent("b"), &ast.Ident{
						Name: "c",
						Node: ast.NewString("foo"),
					},
					ast.NewIdent("c"), ast.NewIdent("d"),
				),
			},
		}},
		want: `a: {
	b: c_9
	c: d
}

let c_9 = c
`,
	}, {
		desc: "Add alias to shadowed field",
		file: func() *ast.File {
			field := &ast.Field{
				Label: ast.NewIdent("a"),
				Value: ast.NewString("b"),
			}
			return &ast.File{Decls: []ast.Decl{
				field,
				&ast.Field{
					Label: ast.NewIdent("c"),
					Value: ast.NewStruct(
						ast.NewIdent("a"), ast.NewStruct(),
						ast.NewIdent("b"), &ast.Ident{
							Name: "a",
							Node: field.Value,
						},
						ast.NewIdent("c"), ast.NewIdent("d"),
					),
				},
			}}
		}(),
		want: `a_9=a: "b"
c: {
	a: {}
	b: a_9
	c: d
}
`,
	}, {
		desc: "Add let clause to shadowed field",
		// Resolve both identifiers to same clause.
		file: func() *ast.File {
			field := &ast.Field{
				Label: ast.NewIdent("a"),
				Value: ast.NewString("b"),
			}
			return &ast.File{Decls: []ast.Decl{
				field,
				&ast.Field{
					Label: ast.NewIdent("c"),
					Value: ast.NewStruct(
						ast.NewIdent("a"), ast.NewStruct(),
						// Remove this reference.
						ast.NewIdent("b"), &ast.Ident{
							Name: "a",
							Node: field.Value,
						},
						ast.NewIdent("c"), ast.NewIdent("d"),
						ast.NewIdent("e"), &ast.Ident{
							Name: "a",
							Node: field.Value,
						},
					),
				},
			}}
		}(),
		want: `a_9=a: "b"
c: {
	a: {}
	b: a_9
	c: d
	e: a_9
}
`,
	}, {
		desc: "Add let clause to shadowed field",
		// Resolve both identifiers to same clause.
		file: func() *ast.File {
			fieldX := &ast.Field{
				Label: &ast.Alias{
					Ident: ast.NewIdent("X"),
					Expr:  ast.NewIdent("a"), // shadowed
				},
				Value: ast.NewString("b"),
			}
			fieldY := &ast.Field{
				Label: &ast.Alias{
					Ident: ast.NewIdent("Y"), // shadowed
					Expr:  ast.NewIdent("q"), // not shadowed
				},
				Value: ast.NewString("b"),
			}
			return &ast.File{Decls: []ast.Decl{
				fieldX,
				fieldY,
				&ast.Field{
					Label: ast.NewIdent("c"),
					Value: ast.NewStruct(
						ast.NewIdent("a"), ast.NewStruct(),
						ast.NewIdent("b"), &ast.Ident{
							Name: "X",
							Node: fieldX,
						},
						ast.NewIdent("c"), ast.NewIdent("d"),
						ast.NewIdent("e"), &ast.Ident{
							Name: "a",
							Node: fieldX.Value,
						},
						ast.NewIdent("f"), &ast.Ident{
							Name: "Y",
							Node: fieldY,
						},
					),
				},
			}}
		}(),
		want: `
let X_9 = X
X=a: "b"
Y=q: "b"
c: {
	a: {}
	b: X
	c: d
	e: X_9
	f: Y
}
`,
	}, {
		desc: "Add let clause to nested shadowed field",
		// Resolve both identifiers to same clause.
		file: func() *ast.File {
			field := &ast.Field{
				Label: ast.NewIdent("a"),
				Value: ast.NewString("b"),
			}
			return &ast.File{Decls: []ast.Decl{
				&ast.Field{
					Label: ast.NewIdent("b"),
					Value: ast.NewStruct(
						field,
						ast.NewIdent("b"), ast.NewStruct(
							ast.NewIdent("a"), ast.NewString("bar"),
							ast.NewIdent("b"), &ast.Ident{
								Name: "a",
								Node: field.Value,
							},
							ast.NewIdent("e"), &ast.Ident{
								Name: "a",
								Node: field.Value,
							},
						),
					),
				},
			}}
		}(),
		want: `b: {
	a_9=a: "b"
	b: {
		a: "bar"
		b: a_9
		e: a_9
	}
}
`,
	}, {
		desc: "Add let clause to nested shadowed field with alias",
		// Resolve both identifiers to same clause.
		file: func() *ast.File {
			field := &ast.Field{
				Label: &ast.Alias{
					Ident: ast.NewIdent("X"),
					Expr:  ast.NewIdent("a"),
				},
				Value: ast.NewString("b"),
			}
			return &ast.File{Decls: []ast.Decl{
				&ast.Field{
					Label: ast.NewIdent("b"),
					Value: ast.NewStruct(
						field,
						ast.NewIdent("b"), ast.NewStruct(
							ast.NewIdent("a"), ast.NewString("bar"),
							ast.NewIdent("b"), &ast.Ident{
								Name: "a",
								Node: field.Value,
							},
							ast.NewIdent("e"), &ast.Ident{
								Name: "a",
								Node: field.Value,
							},
						),
					),
				},
			}}
		}(),
		want: `b: {
	let X_9 = X
	X=a: "b"
	b: {
		a: "bar"
		b: X_9
		e: X_9
	}
}
`,
	}, {
		desc: "Avoid joining file doc comment to added import declaration",
		// Resolve both identifiers to same clause.
		file: func() *ast.File {
			f := &ast.File{
				Decls: []ast.Decl{
					&ast.Field{
						Label: ast.NewIdent("a"),
						Value: ast.NewSel(
							&ast.Ident{
								Name: "list",
								Node: ast.NewImport(nil, "list"),
							},
							"Min",
						),
					},
				},
			}
			// Note: it's important it's not a doc comment, otherwise
			// it gets joined anyway.
			comment := internal.NewComment(true, "file-level comment")
			comment.Doc = false
			ast.SetComments(f, []*ast.CommentGroup{comment})
			return f
		}(),
		want: `// file-level comment

import "list"

a: list.Min
`,
	}}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := astutil.Sanitize(tc.file)
			if err != nil {
				t.Fatal(err)
			}

			b, errs := format.Node(tc.file)
			if errs != nil {
				t.Fatal(errs)
			}

			got := string(b)
			qt.Assert(t, qt.Equals(got, tc.want))
		})
	}
}

// For testing purposes: do not remove.
func TestX(t *testing.T) {
	t.Skip()

	field := &ast.Field{
		Label: &ast.Alias{
			Ident: ast.NewIdent("X"),
			Expr:  ast.NewIdent("a"),
		},
		Value: ast.NewString("b"),
	}

	file := &ast.File{Decls: []ast.Decl{
		&ast.Field{
			Label: ast.NewIdent("b"),
			Value: ast.NewStruct(
				field,
				ast.NewIdent("b"), ast.NewStruct(
					ast.NewIdent("a"), ast.NewString("bar"),
					ast.NewIdent("b"), &ast.Ident{
						Name: "a",
						Node: field.Value,
					},
					ast.NewIdent("e"), &ast.Ident{
						Name: "a",
						Node: field.Value,
					},
				),
			),
		},
	}}

	err := astutil.Sanitize(file)
	if err != nil {
		t.Fatal(err)
	}

	b, errs := format.Node(file)
	if errs != nil {
		t.Fatal(errs)
	}

	t.Error(string(b))
}
