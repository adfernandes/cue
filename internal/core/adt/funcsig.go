// Copyright 2026 CUE Authors
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

package adt

import (
	"fmt"
	"slices"
)

// This file implements unification of CUE function values and
// function types (bodyless function literals).
//
// A function type acts as a constraint on a function value: unifying the two
// yields the original value with the type recorded as an extra [FuncType]
// constraint. Static checks at unification time are limited to what is
// decidable from the compiled signatures alone:
//
//   - every parameter of the type must be declared by the value: named
//     parameters (including a! and a?) match by label, anonymous parameters
//     match the value's positional parameter in the same position; a type
//     parameter that the value does not declare is allowed only if the
//     parameter is optional (a?); callable function values are closed;
//   - a matched pair of parameters must agree on requiredness (a! matches
//     only a!);
//   - unless the type's signature is open, every parameter of the value must
//     be addressable through the type — by label, or positionally for the
//     value's positional parameters — or be optional (a?).
//
// Everything else is deferred to call time, where the type's parameter
// constraints and result constraint are scheduled alongside those of the
// value (see [nodeContext.scheduleFuncCall]). In particular:
//
//   - conflicts between a type's and a value's parameter constraints (e.g.
//     int versus string) surface when the function is called, not when it is
//     unified;
//   - defaults are dynamic and are never probed statically: a value
//     parameter with a default still counts as non-optional for the
//     closedness check above;
//   - the type's parameter order and positional/named calling conventions
//     are not enforced beyond the matching described above; calls are always
//     bound against the value's signature.
//
// Unifying two function types checks the same parameter matching in both
// directions — a parameter present in only one signature is allowed only if
// the other signature is open or the parameter is optional — and yields a
// type carrying both signatures. One rule thus governs extra parameters
// everywhere — for tightening in either direction, for type meets, for
// builtins, and for subsumption (see internal/core/subsume): an extra
// parameter is admitted against a closed signature iff it is optional.
// The meet of the parameter constraints is not materialized: a value unified
// with the result is checked against, and calls are constrained by, each
// recorded signature separately. A parameter that is optional in one
// signature and plain in the other is plain (the stricter form) in effect,
// as calls bind against the value's signature.

// A FuncType records a function type that a function value or another
// function type has been unified with. The type's parameter constraints and
// result constraint must additionally be enforced on any call through the
// carrying [FuncValue].
type FuncType struct {
	Fn  *Function
	Env *Environment
}

// IsFuncType reports whether v is a bodyless function literal, i.e. a
// function type rather than a callable function value. It is exported for
// use by internal/core/subsume.
func IsFuncType(v *FuncValue) bool {
	return v.Fn == nil || v.Fn.Body == nil
}

// MatchFuncParam returns the parameter of fn addressed by the given label,
// or, if label is InvalidLabel, the positional parameter of fn in position
// pos. The second return value is the index of the parameter in fn.Params. It
// is exported for use by internal/core/subsume.
func MatchFuncParam(fn *Function, label Feature, pos int) (FuncParam, int, bool) {
	if label != InvalidLabel {
		for j, q := range fn.Params {
			if q.Label == label {
				return q, j, true
			}
		}
		return FuncParam{}, -1, false
	}
	i := 0
	for j, q := range fn.Params {
		if !q.Positional {
			continue
		}
		if i == pos {
			return q, j, true
		}
		i++
	}
	return FuncParam{}, -1, false
}

// checkParamsDeclared verifies that every parameter of x is declared by y:
// named parameters by label, anonymous parameters by position. A parameter
// of x that y does not declare is allowed only if y's signature is open or
// the parameter is optional (a?): an optional parameter can never be bound
// by a call through y, but no call needs it either. Matched pairs must
// agree on requiredness. The strings xd and yd describe the two signatures
// in error messages.
func checkParamsDeclared(c *OpContext, x, y *Function, xd, yd string) *Bottom {
	pos := 0
	for _, p := range x.Params {
		pi := -1
		if p.Positional {
			pi = pos
			pos++
		}
		var q FuncParam
		var ok bool
		if p.Label != InvalidLabel {
			q, _, ok = MatchFuncParam(y, p.Label, -1)
		} else {
			q, _, ok = MatchFuncParam(y, InvalidLabel, pi)
		}
		if !ok {
			if y.Open || p.ArcType == ArcOptional {
				continue
			}
			if p.Label != InvalidLabel {
				return c.NewErrf("parameter %s of %s not allowed by closed %s",
					p.Label.SelectorString(c), xd, yd)
			}
			return c.NewErrf("%s has more positional parameters than closed %s", xd, yd)
		}
		if (p.ArcType == ArcRequired) != (q.ArcType == ArcRequired) {
			return c.NewErrf("parameter %s must be required in both %s and %s",
				p.Label.SelectorString(c), xd, yd)
		}
	}
	return nil
}

// checkFuncTypeMeet verifies that two function types have unifiable
// signatures: parameters present in both are matched by label or position,
// and a parameter present in only one is allowed only if the other
// signature is open or the parameter is optional.
func checkFuncTypeMeet(c *OpContext, x, y *Function) *Bottom {
	if b := checkParamsDeclared(c, x, y, "function type", "function type"); b != nil {
		return b
	}
	// The reverse direction only checks presence; requiredness of matched
	// pairs was already checked above.
	return checkParamsDeclared(c, y, x, "function type", "function type")
}

// checkFuncTightening verifies that the function value val may be tightened
// by the function type typ: every non-optional parameter of the type must
// be declared by the value, and, unless the type is open, every
// non-optional parameter of the value must be addressable through the type.
func checkFuncTightening(c *OpContext, typ, val *Function) *Bottom {
	if b := checkParamsDeclared(c, typ, val, "function type", "function"); b != nil {
		return b
	}
	if typ.Open {
		return nil
	}
	p, ok := ExtraFuncParam(typ, val)
	if !ok {
		return nil
	}
	if p.Label != InvalidLabel {
		return c.NewErrf("parameter %s of function not allowed by closed function type",
			p.Label.SelectorString(c))
	}
	return c.NewErrf("function has more positional parameters than closed function type")
}

// ExtraFuncParam returns the first non-optional parameter of val that is not
// addressable through the closed signature typ — by label, or, for a
// positional parameter of val, through an anonymous parameter of typ in the
// same position — and reports whether such a parameter exists. An optional
// parameter never qualifies: calls through typ can never bind it, but no
// call needs it either. Note that a parameter with a default does qualify:
// defaults are dynamic and are not probed statically.
//
// The helper implements the closedness side of the one rule governing extra
// parameters (see the file comment) and is shared between function
// tightening and subsumption (internal/core/subsume).
func ExtraFuncParam(typ, val *Function) (FuncParam, bool) {
	pos := 0
	for _, p := range val.Params {
		pi := -1
		if p.Positional {
			pi = pos
			pos++
		}
		if p.Label != InvalidLabel {
			if _, _, ok := MatchFuncParam(typ, p.Label, -1); ok {
				continue
			}
		}
		if pi >= 0 {
			if _, _, ok := MatchFuncParam(typ, InvalidLabel, pi); ok {
				continue
			}
		}
		if p.ArcType == ArcOptional {
			continue
		}
		return p, true
	}
	return FuncParam{}, false
}

// anonParamLabel returns the label of the synthetic canonical-frame slot that
// binds the argument of the anonymous positional parameter at index i of a
// function's parameter list. Anonymous parameters are not referenceable
// from the function body — no compiled reference carries this label — so
// the label only needs to be unique within one call frame: it is a string
// label, which no identifier reference can produce. The arc exists so that
// the parameter's own constraint and any type constraints matched to the
// parameter are enforced on the argument of every call; its name surfaces
// in error paths for a failing argument.
//
// Deriving the label interns a string through the runtime's global index
// lock, so the labels are cached on the OpContext, indexed by parameter
// position: they do not depend on which function is being called.
func anonParamLabel(c *OpContext, i int) Feature {
	for len(c.anonParamLabels) <= i {
		c.anonParamLabels = append(c.anonParamLabels,
			MakeStringLabel(c, fmt.Sprintf("arg%d", len(c.anonParamLabels))))
	}
	return c.anonParamLabels[i]
}

// staticKind returns the kind of a compiled constraint expression when it is
// statically decidable, i.e. when the expression is literally a basic type.
func staticKind(x Expr) (Kind, bool) {
	if t, ok := x.(*BasicType); ok {
		return t.K, true
	}
	return BottomKind, false
}

// kindOnlyConstraint reports the kind a constraint restricts its value to,
// when restricting the kind is all it does. A basic type qualifies by
// definition, and so does an open list whose element constraint is top, as it
// admits every list. Anything that restricts more than the kind — a bound, a
// struct, a list with element constraints or a fixed length — does not, and
// the caller must enforce it.
//
// This is what makes a signature constraint skippable at call time: a builtin
// already knows the kind of each parameter and of its result, and
// [CheckBuiltinTightening] verified those kinds against the type when the two
// were unified. Re-checking a kind-only constraint against the value can only
// succeed, but doing so materializes the value — which for a large list means
// building a vertex per element.
func kindOnlyConstraint(x Expr) (Kind, bool) {
	switch t := x.(type) {
	case *BasicType:
		return t.K, true

	case *Top:
		// `_` admits everything, so it constrains nothing at all.
		return TopKind, true

	case *ListLit:
		// `[...T]` restricts only the kind exactly when T admits everything:
		// `[..._]` and `[...]` do, `[...int]` does not.
		if len(t.Elems) != 1 {
			return BottomKind, false
		}
		e, ok := t.Elems[0].(*Ellipsis)
		if !ok {
			return BottomKind, false
		}
		switch v := e.Value.(type) {
		case nil:
			return ListKind, true
		case *Top:
			return ListKind, true
		case *BasicType:
			if v.K == TopKind {
				return ListKind, true
			}
		}
	}
	return BottomKind, false
}

// CheckBuiltinTightening verifies that builtin b may be tightened by the
// function type typ. It is exported for use by internal/core/subsume, which
// applies the same static check to decide whether a function type subsumes a
// builtin.
//
// Builtins accept no labeled arguments and their parameters are positional
// and unlabeled. Mirroring the anonymous-value rule, a type's anonymous
// parameters match the builtin's parameters positionally; a type's named
// parameters cannot be satisfied by a builtin. The static checks are limited
// to what is decidable from the compiled signature and the builtin's shape:
//
//   - a named parameter of the type is rejected, unless it is optional
//     (a?): an optional parameter can never be bound by a call through the
//     builtin, but no call needs it either;
//   - a type parameter beyond the builtin's parameter count is rejected;
//   - unless the type is open, a defaultless builtin parameter beyond the
//     type's parameter count is rejected, as calls through the type could
//     never provide it. Builtins carry no optional markers; a builtin
//     parameter with a default is admitted instead, since builtin defaults
//     — unlike function value defaults, which are dynamic and never probed
//     — are static metadata;
//   - a parameter or result constraint that is literally a basic type
//     disjoint with the builtin's parameter or result kind is rejected.
//
// All other constraints are enforced dynamically, per call (see
// [Builtin.rawCall]).
func CheckBuiltinTightening(c *OpContext, typ *Function, b *Builtin) *Bottom {
	pos := 0
	for _, p := range typ.Params {
		if p.Label != InvalidLabel {
			if p.ArcType == ArcOptional {
				// An extra parameter is admitted against a closed signature
				// iff it is optional; a builtin never binds named arguments,
				// so the parameter is simply never bound.
				continue
			}
			return c.NewErrf("parameter %s of function type not allowed by builtin %s: builtins accept no labeled arguments",
				p.Label.SelectorString(c), b.qualifiedName(c))
		}
		if pos >= len(b.Params) {
			return c.NewErrf("function type has more positional parameters than builtin %s",
				b.qualifiedName(c))
		}
		if p.Value != nil {
			if k, ok := staticKind(p.Value); ok && k&b.Params[pos].Kind() == BottomKind {
				return c.NewErrf("parameter %d of function type has kind %s, conflicting with kind %s of builtin %s",
					pos+1, k, b.Params[pos].Kind(), b.qualifiedName(c))
			}
		}
		pos++
	}
	if !typ.Open {
		for i := pos; i < len(b.Params); i++ {
			if b.Params[i].Default() == nil {
				return c.NewErrf("builtin %s has more parameters than closed function type",
					b.qualifiedName(c))
			}
		}
	}
	if typ.Ret != nil && b.Result != BottomKind {
		if k, ok := staticKind(typ.Ret); ok && k&b.Result == BottomKind {
			return c.NewErrf("result of function type has kind %s, conflicting with result kind %s of builtin %s",
				k, b.Result, b.qualifiedName(c))
		}
	}
	return nil
}

// mergeBuiltinFunc unifies builtin b with the function value or type f. A
// builtin unifies with a compatible function type, yielding the builtin
// carrying the type as an extra [FuncType] constraint that is additionally
// enforced on every call. A Bottom return describes an incompatible
// signature. A nil, nil return indicates that b and f are conflicting
// values, to be reported like any other conflicting scalars: a builtin
// never unifies with a proper function value.
func mergeBuiltinFunc(c *OpContext, b *Builtin, f *FuncValue) (*Builtin, *Bottom) {
	if !IsFuncType(f) {
		return nil, nil
	}
	add := append([]FuncType{{Fn: f.Fn, Env: f.Env}}, f.Types...)
	types := b.Types
	added := false
	for _, t := range add {
		if slices.Contains(types, t) {
			continue
		}
		if err := CheckBuiltinTightening(c, t.Fn, b); err != nil {
			return nil, err
		}
		for _, u := range types {
			if err := checkFuncTypeMeet(c, t.Fn, u.Fn); err != nil {
				return nil, err
			}
		}
		if !added {
			types = slices.Clone(types)
			added = true
		}
		types = append(types, t)
	}
	if !added {
		return b, nil
	}
	merged := *b
	merged.Types = types
	if merged.orig == nil {
		merged.orig = b
	}
	return &merged, nil
}

// mergeBuiltins unifies two occurrences of the same builtin, merging their
// recorded type constraints. It returns nil if a and b are distinct
// builtins, which conflict like any other distinct scalars.
func mergeBuiltins(a, b *Builtin) *Builtin {
	if a.self() != b.self() {
		return nil
	}
	if len(b.Types) == 0 {
		return a
	}
	if len(a.Types) == 0 {
		return b
	}
	merged := *a
	merged.Types = mergeFuncTypes(a.Types, b.Types)
	return &merged
}

// BuiltinSubsumes reports whether builtin a subsumes builtin b: a builtin
// subsumes itself and any tightened clone of itself, as tightening only adds
// constraints. The two must share their identity — a tightened clone keeps
// the identity of the builtin it was derived from — and every type
// constraint of a must also constrain b. It is exported for use by
// internal/core/subsume.
func BuiltinSubsumes(a, b *Builtin) bool {
	if a.self() != b.self() {
		return false
	}
	for _, t := range a.Types {
		if !slices.Contains(b.Types, t) {
			return false
		}
	}
	return true
}

// mergeFuncValues unifies two function values, types, or a combination
// thereof. It returns the resulting value, or a Bottom describing why the
// signatures are incompatible. A nil, nil return indicates that a and b are
// conflicting function values, to be reported like any other conflicting
// scalars.
func mergeFuncValues(c *OpContext, a, b *FuncValue) (*FuncValue, *Bottom) {
	if !IsFuncType(a) && !IsFuncType(b) {
		// VALUE & VALUE: a function value unifies only with itself. Two
		// partial applications are equal only when they bound the same
		// arguments, and a plain function differs from any partial
		// application of itself, mirroring [equalTerminal].
		if a.Fn != b.Fn || !a.Env.Equal(c, b.Env) || !equalFuncArgs(a.args, b.args) {
			return nil, nil
		}
		merged := *a
		merged.Types = mergeFuncTypes(a.Types, b.Types)
		return &merged, nil
	}

	// At least one of the two is a type. Make prim the value, if any, so
	// that sec is always a type whose signatures are added as constraints.
	prim, sec := a, b
	if IsFuncType(a) && !IsFuncType(b) {
		prim, sec = b, a
	}

	head := FuncType{Fn: prim.Fn, Env: prim.Env}
	types := prim.Types
	added := false
	// The signatures to add are sec itself followed by its recorded types;
	// visit them in place instead of materializing a combined slice.
	for i := -1; i < len(sec.Types); i++ {
		t := FuncType{Fn: sec.Fn, Env: sec.Env}
		if i >= 0 {
			t = sec.Types[i]
		}
		if t == head || slices.Contains(types, t) {
			continue
		}
		if IsFuncType(prim) {
			if b := checkFuncTypeMeet(c, t.Fn, head.Fn); b != nil {
				return nil, b
			}
		} else {
			if b := checkFuncTightening(c, t.Fn, head.Fn); b != nil {
				return nil, b
			}
		}
		for _, u := range types {
			if b := checkFuncTypeMeet(c, t.Fn, u.Fn); b != nil {
				return nil, b
			}
		}
		if !added {
			types = slices.Clone(types)
			added = true
		}
		types = append(types, t)
	}
	if !added {
		return prim, nil
	}
	merged := *prim
	merged.Types = types
	return &merged, nil
}

// equalFuncTypes reports whether a and b record the same set of function
// type constraints. The lists are deduplicated on construction (see
// [mergeFuncTypes]) but ordered by encounter, so equal tightenings may
// record their types in a different order (T1 & T2 & f versus T2 & T1 & f);
// they must still compare equal.
func equalFuncTypes(a, b []FuncType) bool {
	if len(a) != len(b) {
		return false
	}
	for _, t := range a {
		if !slices.Contains(b, t) {
			return false
		}
	}
	return true
}

// equalFuncArgs reports whether two partial applications bound the same
// arguments to the same parameters. Arguments are compared conservatively by
// identity of their expression and environment: distinct bindings such as
// Add(5, ...) and Add(10, ...) are never treated as equal, while an expression
// evaluated in different environments is not assumed equal.
func equalFuncArgs(a, b []funcArg) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if x.expr != b[i].expr || x.env != b[i].env {
			return false
		}
	}
	return true
}

// EqualArgs reports whether the function values x and y bound the same
// arguments to the same parameters by partial application (see
// [equalFuncArgs]). It is exported for use by internal/core/subsume:
// distinct partial applications differ on every call and never subsume one
// another.
func (x *FuncValue) EqualArgs(y *FuncValue) bool {
	return equalFuncArgs(x.args, y.args)
}

// mergeFuncTypes returns the union of two lists of function type
// constraints, preserving the order of a.
func mergeFuncTypes(a, b []FuncType) []FuncType {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		return b
	}
	types := slices.Clone(a)
	for _, t := range b {
		if !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	return types
}
