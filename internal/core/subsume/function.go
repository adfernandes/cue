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

package subsume

import (
	"slices"

	"cuelang.org/go/internal/core/adt"
)

// This file implements subsumption of native CUE function values and
// function types (bodyless function literals).
//
// Subsumption is structural, following the signature matching rules of
// function tightening and function type meets: positional parameters align by
// ordinal, a plain contract label promised by the subsumer must already name
// that slot on the candidate, name-only parameters match by label, and matched
// pairs must agree on requiredness. Directionally, everything the subsumed
// signature declares beyond the subsuming signature is admitted only against an open
// subsuming signature (or when the extra parameter is optional and thus not
// needed for a call to succeed), while the subsumed signature must declare
// every non-optional parameter the subsuming signature declares: an extra
// parameter is admitted against a closed signature iff it is optional, the
// same rule the unification paths apply. Each matched parameter constraint
// of the subsuming signature must subsume the corresponding constraint of
// the subsumed one, and so must the result constraint.
//
// A function type subsumes a (tightened) function value whose signature
// satisfies it. A function value subsumes itself and a tightening of
// itself: tightening only adds constraints. A function type also subsumes a
// (tightened) builtin that its signatures admit, applying the same static
// check the evaluator uses when tightening a builtin; a builtin subsumes
// itself and a tightening of itself (see [adt.BuiltinSubsumes]).

// funcValues reports whether the function value or type a subsumes the
// function value or type b.
func (s *subsumer) funcValues(a, b *adt.FuncValue) bool {
	if adt.IsFuncType(a) {
		// Every signature constraint of a — its own signature and any
		// types it was met with — must admit b.
		if !s.funcSignature(a.Fn, a.Env, b) {
			return false
		}
		for _, t := range a.Types {
			if !s.funcSignature(t.Fn, t.Env, b) {
				return false
			}
		}
		return true
	}

	// A function value subsumes itself and a tightening of itself:
	// tightening only adds constraints, so every type constraint of a must
	// also constrain b. Distinct partial applications differ on every call
	// and never subsume one another: a and b must have bound the same
	// arguments.
	if adt.IsFuncType(b) || a.Fn != b.Fn || !a.Env.Equal(s.ctx, b.Env) || !a.EqualArgs(b) {
		return false
	}
	for _, t := range a.Types {
		if !slices.Contains(b.Types, t) {
			return false
		}
	}
	return true
}

// funcBuiltin reports whether the function value or type a subsumes the
// builtin b. Only a function type can subsume a builtin: a proper function
// value never equals one. The type subsumes b if its own signature and every
// type it was met with admit b, applying the same static check the evaluator
// uses when tightening a builtin (see [adt.CheckBuiltinTightening]).
func (s *subsumer) funcBuiltin(a *adt.FuncValue, b *adt.Builtin) bool {
	if !adt.IsFuncType(a) {
		return false
	}
	if adt.CheckBuiltinTightening(s.ctx, a.Fn, b) != nil {
		return false
	}
	for _, t := range a.Types {
		if adt.CheckBuiltinTightening(s.ctx, t.Fn, b) != nil {
			return false
		}
	}
	return true
}

// funcSignature reports whether the function type signature (fn, env)
// admits the function value or type b.
func (s *subsumer) funcSignature(fn *adt.Function, env *adt.Environment, b *adt.FuncValue) bool {
	// A signature b carries as a recorded type constraint, or that is b's
	// own signature, is satisfied by construction.
	if slices.Contains(b.Types, adt.FuncType{Fn: fn, Env: env}) {
		return true
	}
	if b.Fn == fn && b.Env.Equal(s.ctx, env) {
		return true
	}

	// Every parameter of the signature must be declared by b. Positional
	// parameters align by ordinal; if the signature promises a plain contract
	// label, b must already expose that label on the same slot. Name-only (a!,
	// a?) parameters match by label. Matched pairs must agree on requiredness,
	// and the signature's parameter constraint must subsume b's.
	pos := 0
	for _, p := range fn.Params {
		var q adt.FuncParam
		var ok bool
		if p.Positional {
			q, _, ok = adt.MatchFuncParam(b.Fn, adt.InvalidLabel, pos)
			pos++
			if ok && p.Label != adt.InvalidLabel && q.Label != p.Label {
				return false
			}
			if !ok && p.Label != adt.InvalidLabel {
				// A plain named parameter additionally matches a name-only
				// parameter of b by its label.
				q, _, ok = adt.MatchFuncParam(b.Fn, p.Label, -1)
			}
		} else {
			q, _, ok = adt.MatchFuncParam(b.Fn, p.Label, -1)
		}
		if !ok {
			if p.ArcType == adt.ArcOptional {
				// An extra parameter is admitted against a closed signature
				// iff it is optional, mirroring the tightening and type-meet
				// rules: calls through the signature never need to bind it.
				continue
			}
			return false
		}
		if (p.ArcType == adt.ArcRequired) != (q.ArcType == adt.ArcRequired) {
			return false
		}
		if !s.funcConstraint(env, p.Value, b.Env, q.Value) {
			return false
		}
	}

	// Everything b declares beyond the signature's parameters is admitted
	// only against an open signature. An optional extra parameter is not
	// needed for a call to succeed and is always admitted; the check is the
	// same one the tightening rules apply.
	if !fn.Open {
		if _, ok := adt.ExtraFuncParam(fn, b.Fn); ok {
			return false
		}
	}

	// The signature's result constraint must subsume b's.
	return s.funcConstraint(env, fn.Ret, b.Env, b.Fn.Ret)
}

// funcConstraint reports whether the parameter or result constraint xa,
// compiled in environment envA, subsumes constraint xb compiled in envB. A
// missing constraint is top. Constraints that do not evaluate to a complete
// value are conservatively reported as not subsuming.
func (s *subsumer) funcConstraint(envA *adt.Environment, xa adt.Expr, envB *adt.Environment, xb adt.Expr) bool {
	if xa == nil {
		// Top subsumes everything.
		return true
	}
	va, ok := s.evalFuncConstraint(envA, xa)
	if !ok {
		return false
	}
	if _, isTop := va.(*adt.Top); isTop {
		return true
	}
	var vb adt.Value = &adt.Top{}
	if xb != nil {
		if vb, ok = s.evalFuncConstraint(envB, xb); !ok {
			return false
		}
	}
	return s.values(va, vb)
}

// evalFuncConstraint evaluates a compiled signature constraint in its
// closure environment.
func (s *subsumer) evalFuncConstraint(env *adt.Environment, x adt.Expr) (adt.Value, bool) {
	v, complete := s.ctx.Evaluate(env, x)
	if !complete {
		return nil, false
	}
	if _, ok := v.(*adt.Bottom); ok {
		return nil, false
	}
	return v, true
}
