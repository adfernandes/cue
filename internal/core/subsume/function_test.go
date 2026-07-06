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

package subsume_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/compile"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/subsume"
	"cuelang.org/go/internal/cuetdtest"
	"cuelang.org/go/internal/cuetest"
)

// TestFunctions tests subsumption of native function values and function
// types (bodyless signatures). Subsumption is structural, following the
// signature matching rules of function tightening and type meets: named
// parameters match by label, anonymous parameters match positionally,
// requiredness must agree, and extras of the subsumed signature are
// admitted only against an open subsuming signature. Each matched
// parameter constraint and the result constraint of the subsumer must
// subsume the corresponding constraint of the subsumed.
func TestFunctions(t *testing.T) {
	type subsumeTest struct {
		// the result of b ⊑ a, where a and b are defined in "in"
		err string
		in  string
	}
	const exp = "@experiment(functions)\n"
	testCases := []subsumeTest{
		// Type ⊑ type: open and closed signatures.
		{
			in:  `a: func(n: int, ...) -> int, b: func(n: int, m: int, ...) -> int`,
			err: "",
		},
		{
			// A closed signature does not admit an extra plain parameter.
			in:  `a: func(n: int) -> int, b: func(n: int, m: int) -> int`,
			err: "value not an instance",
		},
		{
			// An optional extra parameter is not needed for a call to
			// succeed and is admitted even by a closed signature.
			in:  `a: func(n: int) -> int, b: func(n: int, m?: int) -> int`,
			err: "",
		},
		{
			// b must declare every parameter a declares.
			in:  `a: func(n: int, m: int, ...) -> int, b: func(n: int) -> int`,
			err: "value not an instance",
		},
		{
			// The same holds when a is closed.
			in:  `a: func(n: int, m: int) -> int, b: func(n: int) -> int`,
			err: "value not an instance",
		},
		{
			// An extra optional parameter of a is not needed for a call to
			// succeed and does not prevent subsumption of a signature that
			// lacks it: an extra parameter is admitted against a closed
			// signature iff it is optional, mirroring the tightening and
			// type-meet rules.
			in:  `a: func(n: int, m?: int) -> int, b: func(n: int) -> int`,
			err: "",
		},
		{
			in:  `a: func(n: int, m?: int, ...) -> int, b: func(n: int) -> int`,
			err: "",
		},
		{
			// The optional-extra rule also admits a function value that does
			// not declare the optional parameter.
			in:  `a: func(n: int, m?: int) -> int, b: func(n: int) -> int: n`,
			err: "",
		},
		{
			in:  `a: func(n: int) -> int, b: func(n: int) -> int`,
			err: "",
		},

		// Parameter widening and narrowing in both directions.
		{
			in:  `a: func(n: number, ...) -> int, b: func(n: int, ...) -> int`,
			err: "",
		},
		{
			in:  `a: func(n: int, ...) -> int, b: func(n: number, ...) -> int`,
			err: "value not an instance",
		},
		{
			in:  `a: func(n: <10, ...) -> int, b: func(n: <5, ...) -> int`,
			err: "",
		},
		{
			in:  `a: func(n: <5, ...) -> int, b: func(n: <10, ...) -> int`,
			err: "value not an instance",
		},

		// Result widening and narrowing in both directions.
		{
			in:  `a: func(n: int, ...) -> number, b: func(n: int, ...) -> int`,
			err: "",
		},
		{
			in:  `a: func(n: int, ...) -> int, b: func(n: int, ...) -> number`,
			err: "value not an instance",
		},

		// Requiredness must agree.
		{
			in:  `a: func(n!: int, ...) -> int, b: func(n: int, ...) -> int`,
			err: "value not an instance",
		},
		{
			in:  `a: func(n: int, ...) -> int, b: func(n!: int, ...) -> int`,
			err: "value not an instance",
		},
		{
			in:  `a: func(n!: int, ...) -> int, b: func(n!: int, ...) -> int`,
			err: "",
		},

		// Anonymous parameters match positionally; named parameters match
		// by label only.
		{
			in:  `a: func(int, ...) -> int, b: func(n: int, ...) -> int`,
			err: "",
		},
		{
			in:  `a: func(int, ...) -> int, b: func(int, ...) -> int`,
			err: "",
		},
		{
			// An anonymous parameter of b cannot be addressed by a's label.
			in:  `a: func(n: int, ...) -> int, b: func(int, ...) -> int`,
			err: "value not an instance",
		},

		// A function value subsumes itself, but not a distinct value with
		// an identical signature.
		{
			in:  `f: func(n: int) -> int: n, a: f, b: f`,
			err: "",
		},
		{
			in:  `f: func(n: int) -> int: n, g: func(n: int) -> int: n, a: f, b: g`,
			err: "value not an instance",
		},

		// A function type subsumes a function value whose signature
		// satisfies it, including a value it tightened; a value subsumes a
		// tightening of itself, but a tightened value does not subsume the
		// plain value.
		{
			in:  `a: func(n: int, ...) -> int, b: func(n: int) -> int: n`,
			err: "",
		},
		{
			in:  `T: func(n: int, ...) -> int, f: func(n: int) -> int: n, a: T, b: T & f`,
			err: "",
		},
		{
			in:  `T: func(n: int, ...) -> int, f: func(n: int) -> int: n, a: f, b: T & f`,
			err: "",
		},
		{
			in:  `T: func(n: <10, ...) -> int, f: func(n: int) -> int: n, a: T & f, b: f`,
			err: "value not an instance",
		},
		{
			// A value does not subsume a type.
			in:  `f: func(n: int) -> int: n, a: f, b: func(n: int, ...) -> int`,
			err: "value not an instance",
		},

		// A partial application subsumes itself, but distinct partial
		// applications differ on every call and never subsume one another.
		// Bound arguments are compared conservatively by identity, so even
		// two separately written applications of the same argument are not
		// treated as equal.
		{
			in:  `f: func(n: int, m: int) -> int: n+m, g: f(5, ...), a: g, b: g`,
			err: "",
		},
		{
			in:  `f: func(n: int, m: int) -> int: n+m, a: f(5, ...), b: f(10, ...)`,
			err: "value not an instance",
		},
		{
			in:  `f: func(n: int, m: int) -> int: n+m, a: f(5, ...), b: f(5, ...)`,
			err: "value not an instance",
		},
		{
			// A plain function value does not subsume a partial application
			// of itself, nor the other way around.
			in:  `f: func(n: int, m: int) -> int: n+m, a: f, b: f(5, ...)`,
			err: "value not an instance",
		},
		{
			in:  `f: func(n: int, m: int) -> int: n+m, a: f(5, ...), b: f`,
			err: "value not an instance",
		},

		// A builtin subsumes itself and a tightening of itself; the
		// tightened builtin does not subsume the plain one.
		{
			in:  "import \"strings\"\na: strings.ToUpper, b: strings.ToUpper",
			err: "",
		},
		{
			in:  "import \"strings\"\nT: func(string, ...) -> string, a: strings.ToUpper, b: T & strings.ToUpper",
			err: "",
		},
		{
			in:  "import \"strings\"\nT: func(string, ...) -> string, a: T & strings.ToUpper, b: strings.ToUpper",
			err: "value not an instance",
		},
		{
			// Distinct builtins do not subsume each other.
			in:  "import \"strings\"\na: strings.ToUpper, b: strings.ToLower",
			err: "value not an instance",
		},

		// A function type subsumes a builtin that satisfies it, including a
		// builtin it tightened, applying the same static signature check
		// used for tightening. A builtin does not subsume a function type.
		{
			in:  "import \"strings\"\na: func(string, ...) -> string, b: strings.ToUpper",
			err: "",
		},
		{
			in:  "import \"strings\"\na: func(string) -> string, b: strings.ToUpper",
			err: "",
		},
		{
			in:  "import \"strings\"\nT: func(string, ...) -> string, a: T, b: T & strings.ToUpper",
			err: "",
		},
		{
			// A parameter kind conflicting with the builtin's is rejected.
			in:  "import \"strings\"\na: func(int, ...) -> string, b: strings.ToUpper",
			err: "value not an instance",
		},
		{
			// So is a conflicting result kind.
			in:  "import \"strings\"\na: func(string, ...) -> int, b: strings.ToUpper",
			err: "value not an instance",
		},
		{
			// A named parameter cannot be satisfied by a builtin.
			in:  "import \"strings\"\na: func(s: string, ...) -> string, b: strings.ToUpper",
			err: "value not an instance",
		},
		{
			in:  "import \"strings\"\na: strings.ToUpper, b: func(string, ...) -> string",
			err: "value not an instance",
		},

		// Functions and non-functions do not subsume each other, except
		// through top.
		{
			in:  `a: func(n: int, ...) -> int, b: 3`,
			err: "value not an instance",
		},
		{
			in:  `f: func(n: int) -> int: n, a: 3, b: f`,
			err: "value not an instance",
		},
		{
			in:  `f: func(n: int) -> int: n, a: _, b: f`,
			err: "",
		},
	}

	cuetdtest.Run(t, testCases, func(t *cuetdtest.T, tc *subsumeTest) {
		t.Update(cuetest.UpdateGoldenFiles())

		// Log descriptive name for debugging
		t.Log(tc.in)

		r := t.M.Runtime()

		file, err := parser.ParseFile("subsume", exp+tc.in)
		if err != nil {
			t.Fatal(err)
		}

		root, errs := compile.Files(nil, r, "", file)
		if errs != nil {
			t.Fatal(errs)
		}

		ctx := eval.NewContext(r, root)
		root.Finalize(ctx)

		// Use low-level lookup to avoid evaluation.
		var a, b adt.Value
		for _, arc := range root.Arcs {
			switch arc.Label {
			case ctx.StringLabel("a"):
				a = arc
			case ctx.StringLabel("b"):
				b = arc
			}
		}

		err = subsume.Value(ctx, a, b)

		var gotErr string
		if err != nil {
			gotErr = strings.TrimSpace(errors.Details(err, nil))
			if strings.Contains(gotErr, "\n") {
				gotErr = "\n" + gotErr
				gotErr = strings.Replace(gotErr, "\n", "\n\t\t\t\t", -1)
			}
		}

		t.Equal(gotErr, tc.err)
	})
}
