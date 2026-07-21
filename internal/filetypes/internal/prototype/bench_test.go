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

package prototype

import (
	"testing"

	"cuelang.org/go/cue/build"
)

// The four D-001 protocol operations (T012), plus the supplemental
// subsidiary-string path, match internal/filetypes/bench_test.go so the
// prototype and tables are directly comparable.

func BenchmarkParseFile(b *testing.B) {
	r, err := loadRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.ParseFile("foo.json", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileAndType(b *testing.B) {
	r, err := loadRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.ParseFileAndType("bar.data", "json+schema", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileAndTypeSubsidiaryString(b *testing.B) {
	r, err := loadRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.ParseFileAndType("foo.x", "code+lang=go", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromFile(b *testing.B) {
	r, err := loadRegistry()
	if err != nil {
		b.Fatal(err)
	}
	f := &build.File{
		Filename: "x.yaml",
		Encoding: build.YAML,
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.FromFile(f, Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseArgs(b *testing.B) {
	r, err := loadRegistry()
	if err != nil {
		b.Fatal(err)
	}
	args := []string{"a.cue", "json:", "b.data", "c.data"}
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.ParseArgs(args); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLoad measures the one-time cost of populating the open
// structures from types.cue with the evaluator — the upper bound on
// what a C4 registration-time evaluation costs per process.
func BenchmarkLoad(b *testing.B) {
	src, err := readTypesCUE()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := Load(src); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegister measures per-registration cost including template
// validation by the evaluator (T013).
func BenchmarkRegister(b *testing.B) {
	src, err := readTypesCUE()
	if err != nil {
		b.Fatal(err)
	}
	decl := []byte(`
encoding: "capnp"
form:     "schema"
stream:   false
`)
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		r, err := Load(src)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := r.Register("capnp", []string{".capnp"}, decl); err != nil {
			b.Fatal(err)
		}
	}
}
