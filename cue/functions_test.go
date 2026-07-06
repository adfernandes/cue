// Copyright 2026 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cue_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/parser"
)

func TestNativeFunctions(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
@experiment(functions)

sum: func(a: int, b: int) -> int: a + b
twice: func(n: int) -> int: sum(n, n)
add1: func(_~x: int) -> int: x + 1
addNamed: func(_~x: int, a: int) -> int: x + a
pick: func(a: int | *5) -> int: a
defaultValue: int | *6
pickRef: func(a: defaultValue) -> int: a
keyword: func(a!: int, b?: int) -> int: a
base: 10
capture: func(a: int) -> int: a + base
paramScope: {
	a: int | *7
	f: func(a: string, b: a) -> int: b
	out: f("shadow")
}

positional: sum(1, 2)
nested:     twice(twice(2))
labeled:    sum(a: 1, b: 2)
aliased:    add1(1)
aliasedBeforeNamed: addNamed(1, 2)
aliasedMixed: addNamed(1, a: 2)
defaulted:  pick()
defaultRef: pickRef()
nameOnly:   keyword(a: 4)
captured:   capture(2)
callerArg: {
	local: 3
	out: capture(local)
}
`)
	if err := v.Err(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path string
		want int64
	}{
		{"positional", 3},
		{"nested", 8},
		{"labeled", 3},
		{"aliased", 2},
		{"aliasedBeforeNamed", 3},
		{"aliasedMixed", 3},
		{"defaulted", 5},
		{"defaultRef", 6},
		{"nameOnly", 4},
		{"captured", 12},
		{"callerArg.out", 13},
		{"paramScope.out", 7},
	} {
		got, err := v.LookupPath(cue.ParsePath(tc.path)).Int64()
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %d; want %d", tc.path, got, tc.want)
		}
	}
}

func TestNativeFunctionErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		err  string
	}{
		{
			name: "return constraint",
			in: `
@experiment(functions)

out: (func() -> string: 1)()
`,
			err: "conflicting values",
		},
		{
			name: "name-only positional",
			in: `
@experiment(functions)

f: func(a!: int) -> int: a
out: f(1)
`,
			err: "missing required argument a",
		},
		{
			name: "missing positional",
			in: `
@experiment(functions)

f: func(int) -> int: 1
out: f()
`,
			err: "not enough arguments in function call",
		},
		{
			name: "missing named",
			in: `
@experiment(functions)

f: func(a: int) -> int: a
out: f()
`,
			err: "missing argument a",
		},
		{
			name: "labeled builtin",
			in: `
@experiment(functions)

out: len(x: "foo")
`,
			err: "labeled arguments are not supported for builtin",
		},
		{
			name: "positional after labeled call",
			in: `
@experiment(functions)

sum: func(a: int, b: int) -> int: a + b
out: sum(a: 1, 2)
`,
			err: "positional argument after labeled argument",
		},
		{
			name: "duplicate positional and labeled",
			in: `
@experiment(functions)

sum: func(a: int, b: int) -> int: a + b
out: sum(1, a: 2)
`,
			err: "argument a provided by position and label",
		},
		{
			name: "return scope",
			in: `
@experiment(functions)

a: string
f: func(a: int) -> a: a
out: f(1)
`,
			err: "conflicting values",
		},
		{
			name: "anonymous after named",
			in: `
@experiment(functions)

bad: func(a: int, int) -> int: 1
out: bad(1, 2)
`,
			err: "positional parameter after named parameter",
		},
		{
			name: "alias-bound positional after named",
			in: `
@experiment(functions)

bad: func(a: int, _~x: int) -> int: x
out: bad(1, 2)
`,
			err: "positional parameter after named parameter",
		},
		{
			name: "duplicate parameter",
			in: `
@experiment(functions)

bad: func(a: int, a: int) -> int: a
out: bad(1, 2)
`,
			err: "redeclared",
		},
		{
			name: "dual alias parameter",
			in: `
@experiment(functions)

bad: func(_~(K,V): int) -> int: V
out: bad(1)
`,
			err: "dual postfix alias not supported in function parameters",
		},
		{
			name: "recursive default",
			in: `
@experiment(functions)

f: func(n: int | *f()) -> int: n
out: f()
`,
			err: "missing argument n",
		},
		{
			name: "mutual recursive default",
			in: `
@experiment(functions)

f: func(n: int | *g()) -> int: n
g: func(n: int | *f()) -> int: n
out: f()
`,
			err: "missing argument n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(tc.in)
			err := v.Err()
			if err == nil {
				err = v.LookupPath(cue.ParsePath("out")).Err()
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !strings.Contains(got, tc.err) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNativeFunctionsRequireExperiment(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
sum: func(a: int, b: int) -> int: a + b
`)
	err := v.Err()
	if err == nil {
		t.Fatal("expected missing experiment error")
	}
	if got := err.Error(); !strings.Contains(got, "function syntax requires @experiment(functions)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNativeFunctionProgrammaticCallArgOrder(t *testing.T) {
	f, err := parser.ParseFile("input.cue", `
@experiment(functions)

sum: func(a: int, b: int) -> int: a + b
out: sum(1, b: 2)
`, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}

	var call *ast.CallExpr
	ast.Walk(f, func(n ast.Node) bool {
		if x, ok := n.(*ast.CallExpr); ok {
			call = x
			return false
		}
		return true
	}, nil)
	if call == nil {
		t.Fatal("missing call expression")
	}
	if len(call.ArgLabels) != 2 || call.ArgLabels[1] == nil {
		t.Fatalf("unexpected call labels: %#v", call.ArgLabels)
	}
	call.ArgLabels[0], call.ArgLabels[1] = call.ArgLabels[1], nil

	v := cuecontext.New().BuildFile(f)
	err = v.Err()
	if err == nil {
		t.Fatal("expected call argument order error")
	}
	if got := err.Error(); !strings.Contains(got, "positional argument after labeled argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFuncIdentifierWithoutFunctionsExperiment(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
func: len
out: func("foo")
`)
	if err := v.Err(); err != nil {
		t.Fatal(err)
	}
	got, err := v.LookupPath(cue.ParsePath("out")).Int64()
	if err != nil {
		t.Fatal(err)
	}
	if got != 3 {
		t.Fatalf("got %d; want 3", got)
	}
}

// TestCallUnresolvedDisjunction checks that calling a disjunction that has
// not resolved to a single callee reports a graceful incomplete error. This
// used to panic with a nil-pointer dereference when the callee evaluated to
// no single value.
func TestCallUnresolvedDisjunction(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		err  string
	}{
		{
			name: "functions",
			in: `
@experiment(functions)

f: func(a: int) -> int: a
g: func(a: int) -> int: a + 1
h: f | g
out: h(1)
`,
			err: "unresolved disjunction",
		},
		{
			name: "builtins",
			in: `
import "strings"

h: strings.ToUpper | strings.ToLower
out: h("Hello")
`,
			err: "unresolved disjunction",
		},
		{
			name: "mixed function and non-function",
			in: `
@experiment(functions)

f: func(a: int) -> int: a
h: f | 1
out: h(1)
`,
			err: "unresolved disjunction",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(tc.in)
			out := v.LookupPath(cue.ParsePath("out"))
			err := out.Err()
			if err == nil {
				// An unresolved disjunction is an incomplete error, which
				// Err may not report; validating concreteness must.
				err = out.Validate(cue.Concrete(true))
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !strings.Contains(got, tc.err) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	// A disjunction with a default of function kind resolves to its
	// default and the call proceeds.
	t.Run("default resolves", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`
@experiment(functions)

f: func(a: int) -> int: a
g: func(a: int) -> int: a + 1
h: *f | g
out: h(1)
`)
		if err := v.Err(); err != nil {
			t.Fatal(err)
		}
		got, err := v.LookupPath(cue.ParsePath("out")).Int64()
		if err != nil {
			t.Fatal(err)
		}
		if got != 1 {
			t.Fatalf("got %d; want 1", got)
		}
	})
}

// TestNativeFunctionRecursionIsCycle checks that recursive and mutually
// recursive function calls terminate as structural cycles rather than looping.
// Native function application is sugar over the `(f & {...}).out` struct
// pattern, so recursion re-references a shared template vertex and is caught by
// the regular structural cycle detector. Crucially, non-recursive nesting such
// as twice(twice(2)) must not be misflagged.
func TestNativeFunctionRecursionIsCycle(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{
			name: "direct",
			in: `
@experiment(functions)

f: func(n: int) -> int: f(n)
out: f(1)
`,
		},
		{
			name: "mutual",
			in: `
@experiment(functions)

f: func(n: int) -> int: g(n)
g: func(n: int) -> int: f(n)
out: f(1)
`,
		},
		{
			name: "fibonacci",
			in: `
@experiment(functions)

fib: func(n: int) -> int: fib(n-1) + fib(n-2)
out: fib(5)
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(tc.in)
			err := v.LookupPath(cue.ParsePath("out")).Err()
			if err == nil {
				t.Fatal("expected recursive function call to fail")
			}
			if got := err.Error(); !strings.Contains(got, "structural cycle") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestNativeFunctionNestingIsNotCycle ensures non-recursive nesting of calls to
// the same function is not misflagged as a cycle by the structural cycle
// detector. This is the false-positive guard for collapsing all calls to a
// single identity: twice(twice(2)) must equal 8. The deeper variants guard
// against the callee's cycle-reference chain leaking into the lazily
// evaluated arguments, which made a single call site inside a helper falsely
// re-appear in one reference chain at nesting depth >= 3.
func TestNativeFunctionNestingIsNotCycle(t *testing.T) {
	const prelude = `
@experiment(functions)

sum:   func(a: int, b: int) -> int: a + b
twice: func(n: int) -> int: sum(n, n)
h:     func(n: int) -> int: twice(n)
f:     func(v: int) -> int: v
`
	for _, tc := range []struct {
		name string
		expr string
		want int64
	}{
		{"depth2", "twice(twice(2))", 8},
		{"depth3", "twice(twice(twice(2)))", 16},
		{"depth4", "twice(twice(twice(twice(2))))", 32},
		{"helper", "h(h(h(1)))", 8},
		{"identity", "f(f(f(3)))", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(prelude + "out: " + tc.expr + "\n")
			if err := v.Err(); err != nil {
				t.Fatal(err)
			}
			got, err := v.LookupPath(cue.ParsePath("out")).Int64()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("%s: got %d; want %d", tc.expr, got, tc.want)
			}
		})
	}

	// A cycle must not be misattributed to an unrelated intermediate field
	// that is consumed as an argument.
	t.Run("intermediate", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(prelude + `
mid:   twice(2)
outer: twice(twice(mid))
`)
		if err := v.Err(); err != nil {
			t.Fatal(err)
		}
		for _, e := range []struct {
			path string
			want int64
		}{
			{"mid", 4},
			{"outer", 16},
		} {
			got, err := v.LookupPath(cue.ParsePath(e.path)).Int64()
			if err != nil {
				t.Fatal(err)
			}
			if got != e.want {
				t.Fatalf("%s: got %d; want %d", e.path, got, e.want)
			}
		}
	})
}

// TestNativeFunctionsInDisjunction ensures function calls inside disjunction
// operands take the regular cycle rules rather than the optional-conjunct
// affordances of the structural cycle detector. The affordances assume that
// re-entering an arc settles to a fixed point; a call re-instantiates its
// template instead, so skipping the conjunct used to drop the call's result
// (losing the field) or loop forever (recursion re-afforded each level).
func TestNativeFunctionsInDisjunction(t *testing.T) {
	t.Run("recursive body disjunct is eliminated", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`
@experiment(functions)

f:   func(n: int) -> int: n | f(n)
out: f(1)
`)
		if err := v.Err(); err != nil {
			t.Fatal(err)
		}
		got, err := v.LookupPath(cue.ParsePath("out")).Int64()
		if err != nil {
			t.Fatal(err)
		}
		if got != 1 {
			t.Fatalf("recursive disjunct should be eliminated: got %d; want 1", got)
		}
	})

	t.Run("recursion fails the branch", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`
@experiment(functions)

x: {
	loop: func(n: int) -> int: loop(n)
	out:  loop(1)
} | {out: 7}
`)
		if err := v.Err(); err != nil {
			t.Fatal(err)
		}
		got, err := v.LookupPath(cue.ParsePath("x.out")).Int64()
		if err != nil {
			t.Fatal(err)
		}
		if got != 7 {
			t.Fatalf("recursive branch should fail as a cycle, selecting 7; got %d", got)
		}
	})

	t.Run("nesting evaluates in the branch", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`
@experiment(functions)

x: {
	sum:   func(a: int, b: int) -> int: a + b
	twice: func(n: int) -> int: sum(n, n)
	out:   twice(twice(2))
} | {out: 3, bad: 1 & 2}
`)
		if err := v.Err(); err != nil {
			t.Fatal(err)
		}
		got, err := v.LookupPath(cue.ParsePath("x.out")).Int64()
		if err != nil {
			t.Fatal(err)
		}
		if got != 8 {
			t.Fatalf("twice(twice(2)) in winning disjunct: got %d; want 8", got)
		}
	})
}

// TestNativeFunctionErrorsHideInternals ensures that errors from function
// calls do not leak the internal result label of the struct-based call
// representation: that a call is evaluated as a struct is an implementation
// detail that must not surface in error messages or paths.
func TestNativeFunctionErrorsHideInternals(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "recursion",
			in: `
@experiment(functions)

fn:  func(n: int) -> int: fn(n)
out: fn(1)
`,
			want: "structural cycle",
		},
		{
			name: "return constraint conflict",
			in: `
@experiment(functions)

out: (func() -> string: 1)()
`,
			want: "conflicting values",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(tc.in)
			err := v.Validate(cue.Concrete(true))
			if err == nil {
				t.Fatal("expected error")
			}
			got := errors.Details(err, nil)
			if !strings.Contains(got, tc.want) {
				t.Errorf("error does not contain %q:\n%s", tc.want, got)
			}
			if strings.Contains(got, "_out") {
				t.Errorf("error leaks internal label _out:\n%s", got)
			}
		})
	}
}
