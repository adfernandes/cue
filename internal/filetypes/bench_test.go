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

// These benchmarks cover the four representative operations recorded in
// specs/ace8n-dynamic-filetypes research.md §3. Warm benchmarks measure the
// memoized steady state. Cold benchmarks pre-materialize the built-in data and
// clear the relevant cache before each operation outside the timed region, so
// they measure a cache miss, the generic driver, and the cache fill without
// charging either one-time setup or the test-only invalidation mechanism. The
// subsidiary-string benchmark covers the intentionally uncached
// arbitrary-value path. BenchmarkBuiltinDataMaterialization separately
// measures the one-time construction now deferred from package initialization.
//
// Run warm benchmarks with -benchtime=2s. Run Cold benchmarks with a fixed
// -benchtime=10000x so repeated timer stops do not make the wall-clock run
// unnecessarily long. Results are comparable only on the same machine.

func BenchmarkParseFile(b *testing.B) {
	clearToFileCache()
	if _, err := ParseFile("foo.json", Input); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := ParseFile("foo.json", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileCold(b *testing.B) {
	_ = builtinRegistry()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		clearToFileCache()
		b.StartTimer()
		if _, err := ParseFile("foo.json", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileAndType(b *testing.B) {
	clearParsedScopeCache()
	clearToFileCache()
	if _, err := ParseFileAndType("bar.data", "json+schema", Input); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := ParseFileAndType("bar.data", "json+schema", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileAndTypeCold(b *testing.B) {
	_ = tagTypes()
	_ = builtinRegistry()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		clearParsedScopeCache()
		clearToFileCache()
		b.StartTimer()
		if _, err := ParseFileAndType("bar.data", "json+schema", Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFileAndTypeSubsidiaryString(b *testing.B) {
	_ = tagTypes()
	_ = builtinRegistry()
	b.ResetTimer()
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
	fromFileCache.Clear()
	if _, err := FromFile(f, Input); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := FromFile(f, Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromFileCold(b *testing.B) {
	f := &build.File{
		Filename: "x.yaml",
		Encoding: build.YAML,
	}
	_ = builtinRegistry()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		fromFileCache.Clear()
		b.StartTimer()
		if _, err := FromFile(f, Input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseArgs(b *testing.B) {
	args := []string{"a.cue", "json:", "b.data", "c.data"}
	clearParsedScopeCache()
	clearToFileCache()
	if _, err := ParseArgs(args); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := ParseArgs(args); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseArgsCold(b *testing.B) {
	args := []string{"a.cue", "json:", "b.data", "c.data"}
	_ = tagTypes()
	_ = builtinRegistry()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		clearParsedScopeCache()
		clearToFileCache()
		b.StartTimer()
		if _, err := ParseArgs(args); err != nil {
			b.Fatal(err)
		}
	}
}

var (
	benchmarkTagTypes        map[string]TagType
	benchmarkBuiltinRegistry *registryData
)

// BenchmarkBuiltinDataMaterialization measures the fresh construction
// performed once by the sync.OnceValue accessors. Calling the generated
// factories directly keeps the cost repeatable within one benchmark process.
func BenchmarkBuiltinDataMaterialization(b *testing.B) {
	b.Run("TagTypes", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkTagTypes = newTagTypes()
		}
	})
	b.Run("Registry", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkBuiltinRegistry = newBuiltinRegistry()
		}
	})
	b.Run("Both", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkTagTypes = newTagTypes()
			benchmarkBuiltinRegistry = newBuiltinRegistry()
		}
	})
}
