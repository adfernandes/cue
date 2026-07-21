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

package filetypes

import (
	"testing"

	"cuelang.org/go/cue/build"
)

// These benchmarks are the fixed measurement protocol for file-type
// resolution performance (specs/ace8n-dynamic-filetypes, D-001). They
// cover the four representative operations recorded in that change's
// research.md §3: the common extension-only path, a tag-qualified path,
// FromFile, and a mixed ParseArgs invocation. The subsidiary-string
// benchmark supplements that protocol with a path whose arbitrary tag
// value cannot be covered by a finite lookup table or result cache. Run
// with -benchtime=2s when comparing implementations; results are only
// comparable on the same machine.

func BenchmarkParseFile(b *testing.B) {
	for b.Loop() {
		if _, err := ParseFile("foo.json", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileAndType(b *testing.B) {
	for b.Loop() {
		if _, err := ParseFileAndType("bar.data", "json+schema", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileAndTypeSubsidiaryString(b *testing.B) {
	for b.Loop() {
		if _, err := ParseFileAndType("foo.x", "code+lang=go", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromFile(b *testing.B) {
	f := &build.File{
		Filename: "x.yaml",
		Encoding: build.YAML,
	}
	for b.Loop() {
		if _, err := FromFile(f, Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseArgs(b *testing.B) {
	args := []string{"a.cue", "json:", "b.data", "c.data"}
	for b.Loop() {
		if _, err := ParseArgs(args); err != nil {
			b.Fatal(err)
		}
	}
}
