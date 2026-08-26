// Copyright 2019 CUE Authors
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

package jsonschema

import (
	"strconv"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

// Array constraints

func constraintAdditionalItems(key string, n cue.Value, s *state) {
	var elem ast.Expr
	switch n.Kind() {
	case cue.BoolKind:
		// Boolean values are supported even in earlier
		// versions that did not support boolean schemas otherwise.
		elem = boolSchema(s.boolValue(n))
	case cue.StructKind:
		elem = s.schema(n)
	default:
		s.errf(n, `value of "additionalItems" must be an object or boolean`)
	}
	if !s.hasListPrefix() || !s.listItemsIsArray {
		// If there's no "items" keyword or its value is not an array "additionalItems" doesn't apply.
		return
	}
	setAdditionalItems(n, elem, s)
}

// setAdditionalItems constrains the items beyond the prefix established
// by "items" (pre-2020-12) or "prefixItems" (2020-12+). It handles the
// "additionalItems" keyword (pre-2020-12) and the "items" keyword
// (2020-12+) when prefixItems is present.
func setAdditionalItems(n cue.Value, elem ast.Expr, s *state) {
	if s.listStruct != nil {
		setAdditionalItemsStruct(n, elem, s)
		return
	}
	if len(s.list.Elts) == 0 {
		// Should never happen because prefixItems always adds an ellipsis
		panic("no elements in list")
	}
	last := s.list.Elts[len(s.list.Elts)-1].(*ast.Ellipsis)
	if isErrorCall(elem) {
		// No additional elements allowed. Remove the ellipsis.
		s.list.Elts = s.list.Elts[:len(s.list.Elts)-1]
		return
	}
	if isTop(elem) {
		// Nothing to do: there's already an ellipsis in place that
		// allows anything.
		return
	}
	last.Type = elem
}

// setAdditionalItemsStruct is the [setAdditionalItems] counterpart for
// the index-constraint form of the prefix (see [constraintPrefixItems]).
func setAdditionalItemsStruct(n cue.Value, elem ast.Expr, s *state) {
	if isTop(elem) {
		// Nothing to do: the embedded open list already allows anything.
		return
	}
	if isErrorCall(elem) {
		// No additional elements allowed, so bound the length instead:
		// there's no list literal to close.
		list := s.addImport(n, "list")
		s.add(n, arrayType, ast.NewCall(ast.NewSel(list, "MaxItems"),
			ast.NewLit(token.INT, strconv.Itoa(s.listPrefixLen))))
		return
	}
	s.listStruct.Elts = append(s.listStruct.Elts, &ast.Field{
		Label: indexPattern(&ast.UnaryExpr{
			Op: token.GEQ,
			X:  ast.NewLit(token.INT, strconv.Itoa(s.listPrefixLen)),
		}),
		Value: elem,
	})
}

// indexPattern returns the label for a list pattern constraint
// matching the indexes described by x.
func indexPattern(x ast.Expr) ast.Label {
	return &ast.ListLit{Elts: []ast.Expr{x}}
}

func constraintMinContains(key string, n cue.Value, s *state) {
	p, err := uint64Value(n)
	if err != nil {
		s.errf(n, `value of "minContains" must be a non-negative integer value`)
		return
	}
	s.minContains = &p
}

func constraintMaxContains(key string, n cue.Value, s *state) {
	p, err := uint64Value(n)
	if err != nil {
		s.errf(n, `value of "maxContains" must be a non-negative integer value`)
		return
	}
	s.maxContains = &p
}

func constraintContains(key string, n cue.Value, s *state) {
	list := s.addImport(n, "list")
	x := s.schema(n)

	var min uint64 = 1
	if s.minContains != nil {
		min = *s.minContains
	}
	var c ast.Expr = &ast.UnaryExpr{
		Op: token.GEQ,
		X:  ast.NewLit(token.INT, strconv.FormatUint(min, 10)),
	}

	if s.maxContains != nil {
		c = ast.NewBinExpr(token.AND, c, &ast.UnaryExpr{
			Op: token.LEQ,
			X:  ast.NewLit(token.INT, strconv.FormatUint(*s.maxContains, 10)),
		})
	}

	x = ast.NewCall(ast.NewSel(list, "MatchN"), c, clearPos(x))
	s.add(n, arrayType, x)
}

func constraintItems(key string, n cue.Value, s *state) {
	switch n.Kind() {
	case cue.StructKind, cue.BoolKind:
		elem := s.schema(n)
		ast.SetRelPos(elem, token.NoRelPos)
		if s.hasListPrefix() {
			// In draft2020-12, when prefixItems is present, "items" applies
			// only to elements beyond the prefix, like "additionalItems"
			// did in earlier versions.
			setAdditionalItems(n, elem, s)
		} else {
			s.add(n, arrayType, ast.NewList(&ast.Ellipsis{Type: elem}))
		}
		s.hasItems = true

	case cue.ListKind:
		if !s.schemaVersion.is(vto(VersionDraft2019_09)) {
			// The list form is only supported up to 2019-09
			s.errf(n, `from version %v onwards, the value of "items" must be an object or a boolean`, VersionDraft2020_12)
			return
		}
		s.listItemsIsArray = true
		constraintPrefixItems(key, n, s)
	}
}

func constraintPrefixItems(key string, n cue.Value, s *state) {
	if n.Kind() != cue.ListKind {
		s.errf(n, `value of "prefixItems" must be an array`)
	}
	var a []ast.Expr
	for _, n := range s.listItems(key, n, true) {
		v := s.schema(n)
		ast.SetRelPos(v, token.NoRelPos)
		a = append(a, v)
	}
	s.listPrefixLen = len(a)
	if uint64(len(a)) <= s.minItems {
		// Every element of the prefix is guaranteed to be present,
		// so a regular CUE list literal expresses the constraint exactly.
		s.list = ast.NewList(a...)
		s.list.Elts = append(s.list.Elts, &ast.Ellipsis{})
		s.add(n, arrayType, s.list)
		return
	}
	// The instance is allowed to be shorter than the prefix, and a
	// positional schema applies only when an element is actually
	// present at that index. A list literal cannot express that, but
	// an index pattern constraint can, because it holds vacuously
	// when the list is too short.
	decls := make([]ast.Decl, 0, len(a)+1)
	decls = append(decls, &ast.EmbedDecl{Expr: ast.NewList(&ast.Ellipsis{})})
	for i, elem := range a {
		decls = append(decls, &ast.Field{
			Label: indexPattern(ast.NewLit(token.INT, strconv.Itoa(i))),
			Value: elem,
		})
	}
	s.listStruct = &ast.StructLit{Elts: decls}
	s.add(n, arrayType, s.listStruct)
}

func constraintMaxItems(key string, n cue.Value, s *state) {
	list := s.addImport(n, "list")
	x := ast.NewCall(ast.NewSel(list, "MaxItems"), clearPos(s.uint(n)))
	s.add(n, arrayType, x)
}

func constraintMinItems(key string, n cue.Value, s *state) {
	a := []ast.Expr{}
	p, err := uint64Value(n)
	if err != nil {
		s.errf(n, "invalid uint")
	}
	s.minItems = p
	for ; p > 0; p-- {
		a = append(a, top())
	}
	s.add(n, arrayType, ast.NewList(append(a, &ast.Ellipsis{})...))

	// TODO: use this once constraint resolution is properly implemented.
	// list := s.addImport(n, "list")
	// s.addConjunct(n, ast.NewCall(ast.NewSel(list, "MinItems"), clearPos(s.uint(n))))
}

func constraintUniqueItems(key string, n cue.Value, s *state) {
	if s.boolValue(n) {
		if s.schemaVersion.is(k8s) {
			s.errf(n, "cannot set uniqueItems to true in a Kubernetes schema")
			return
		}
		list := s.addImport(n, "list")
		s.add(n, arrayType, ast.NewCall(ast.NewSel(list, "UniqueItems")))
	}
}

func clearPos(e ast.Expr) ast.Expr {
	ast.SetRelPos(e, token.NoRelPos)
	return e
}
