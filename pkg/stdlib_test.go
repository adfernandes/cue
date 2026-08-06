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

package pkg_test

import (
	iofs "io/fs"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal"
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/pkg"
	"github.com/go-quicktest/qt"
)

func TestImportPaths(t *testing.T) {
	paths := pkg.ImportPaths()
	qt.Assert(t, qt.IsTrue(len(paths) > 0), qt.Commentf("no packages embedded"))
	qt.Assert(t, qt.IsTrue(slices.IsSorted(paths)), qt.Commentf("import paths are not sorted: %v", paths))
	for _, ip := range []string{"strings", "encoding/json", "tool/cli"} {
		qt.Check(t, qt.IsTrue(slices.Contains(paths, ip)), qt.Commentf("import paths are missing %q: %v", ip, paths))
	}
	_, ok := pkg.Source("no/such/package")
	qt.Assert(t, qt.IsFalse(ok), qt.Commentf("Source succeeded for a package that does not exist"))
}

// TestEmbedComplete checks that every pkg.cue file on disk is
// embedded: the go:embed patterns name each directory depth
// explicitly, so a definition file at a new depth would otherwise be
// silently omitted.
func TestEmbedComplete(t *testing.T) {
	var onDisk []string
	err := filepath.WalkDir(".", func(filename string, d iofs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "pkg.cue" {
			return err
		}
		onDisk = append(onDisk, filepath.ToSlash(filename))
		return nil
	})
	qt.Assert(t, qt.IsNil(err))
	slices.Sort(onDisk)

	var embedded []string
	for _, ip := range pkg.ImportPaths() {
		embedded = append(embedded, ip+"/pkg.cue")
	}
	slices.Sort(embedded)
	qt.Assert(t, qt.DeepEquals(embedded, onDisk))
}

var (
	funcSig      = regexp.MustCompile(`^func\((.*)\) -> (.+)$`)
	validatorSig = regexp.MustCompile(`^validator\(.+\)$`)
)

// TestDefsMatchRegisteredPackages checks each pkg.cue file against
// the builtin package it describes: they declare the same members,
// functions are declared as required fields, and the sig attributes
// agree with the registered builtin functions on which members are
// functions, their number of parameters, and which parameters carry
// defaults. A sig may use a validator form only when the evaluator
// treats the builtin as a validator; the reverse is not enforced —
// the evaluator promotes any builtin with a bool result, while the
// validator forms follow intent, which the Go source declares with a
// pkg.Validator result.
func TestDefsMatchRegisteredPackages(t *testing.T) {
	r := runtime.New()
	ctx := cuecontext.New()
	for _, ip := range pkg.ImportPaths() {
		t.Run(ip, func(t *testing.T) {
			src, ok := pkg.Source(ip)
			qt.Assert(t, qt.IsTrue(ok), qt.Commentf("no source for %q", ip))
			f, err := parser.ParseFile(ip+"/pkg.cue", src, parser.ParseComments)
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.CmpEquals(f.PackageName(), path.Base(ip)))
			// The definitions must be well-formed CUE, not merely
			// syntactically valid.
			qt.Check(t, qt.IsNil(ctx.BuildFile(f).Err()), qt.Commentf("definitions do not build"))

			vertex := r.LoadBuiltin(ip)
			qt.Assert(t, qt.IsNotNil(vertex), qt.Commentf("%q is not a registered builtin package", ip))

			// A name may be declared by several fields, such as the
			// per-member declarations of uuid's variants; the sig
			// attribute can only be on the first.
			fields := make(map[string]*ast.Field)
			for _, decl := range f.Decls {
				field, ok := decl.(*ast.Field)
				if !ok {
					continue
				}
				name, _, err := ast.LabelName(field.Label)
				qt.Assert(t, qt.IsNil(err), qt.Commentf("cannot name label of %v", field.Label))
				if _, ok := fields[name]; !ok {
					fields[name] = field
				}
			}
			arcs := make(map[string]*adt.Vertex)
			for _, arc := range vertex.Arcs {
				arcs[arc.Label.SelectorString(r)] = arc
			}

			declared := slices.Sorted(maps.Keys(fields))
			registered := slices.Sorted(maps.Keys(arcs))
			qt.Check(t, qt.DeepEquals(declared, registered))

			for name, field := range fields {
				arc, ok := arcs[name]
				if !ok {
					continue // already reported above
				}
				// A builtin usable bare as a validator is registered
				// as one.
				var builtin *adt.Builtin
				bare := false
				switch v := arc.BaseValue.(type) {
				case *adt.Builtin:
					builtin = v
				case *adt.BuiltinValidator:
					builtin = v.Builtin
					bare = true
				}
				isFunc := builtin != nil
				sig, isSig := stdlibSig(t, field)
				if !qt.Check(t, qt.Equals(isSig, isFunc),
					qt.Commentf("%s: sig attribute presence vs being a function", name)) {
					continue
				}
				// A function's value cannot be carried by the
				// definition file, so it is declared as a required
				// field.
				qt.Check(t, qt.Equals(field.Constraint == token.NOT, isFunc),
					qt.Commentf("%s: required marker vs being a function", name))
				if !isFunc {
					continue
				}
				if validatorSig.MatchString(sig) {
					qt.Check(t, qt.IsTrue(bare),
						qt.Commentf("%s: bare validator sig %q, but not registered as a validator", name, sig))
					continue
				}
				m := funcSig.FindStringSubmatch(sig)
				if !qt.Check(t, qt.IsNotNil(m), qt.Commentf("%s: malformed sig %q", name, sig)) {
					continue
				}
				var params []string
				if m[1] != "" {
					params = strings.Split(m[1], ", ")
				}
				// A validator applying to partial calls leaves its
				// first parameter to the validated value; the sig
				// declares the remaining parameters.
				partial := validatorSig.MatchString(m[2])
				if partial {
					qt.Check(t, qt.IsTrue(!bare && builtin.IsValidator(len(builtin.Params)-1)),
						qt.Commentf("%s: validator result, but the builtin does not validate partial calls, sig %q", name, sig))
				}
				offset := 0
				if partial {
					offset = 1
				}
				if !qt.Check(t, qt.HasLen(params, len(builtin.Params)-offset),
					qt.Commentf("%s: sig parameters vs builtin parameters", name)) {
					continue
				}
				for i, p := range params {
					hasDefault := builtin.Params[i+offset].Default() != nil
					qt.Check(t, qt.Equals(strings.Contains(p, "| *"), hasDefault),
						qt.Commentf("%s: parameter %q default vs builtin parameter %d having one", name, p, i+offset))
				}
			}
		})
	}
}

// TestValidatorSigDetection pins one validator-form signature. The
// generator detects validator intent through the pkg.Validator result
// alias in the Go signatures; were that detection to degrade, every
// validator form would revert to a plain function signature while
// every other check in this package still passed.
func TestValidatorSigDetection(t *testing.T) {
	src, ok := pkg.Source("strings")
	qt.Assert(t, qt.IsTrue(ok))
	qt.Assert(t, qt.StringContains(string(src),
		"MinRunes!: _ @stdlib(func(_~min: int) -> validator(string))"))
}

// stdlibSig reports the signature recorded by a field's @stdlib
// attribute, and whether the field carries one at all.
func stdlibSig(t *testing.T, field *ast.Field) (string, bool) {
	for _, attr := range field.Attrs {
		a := internal.ParseAttr(attr)
		if a.Name != "stdlib" {
			continue
		}
		qt.Assert(t, qt.IsNil(a.Err), qt.Commentf("malformed attribute %q", attr.Text))
		sig, err := a.String(0)
		qt.Assert(t, qt.IsNil(err), qt.Commentf("attribute %q carries no signature", attr.Text))
		return sig, true
	}
	return "", false
}
