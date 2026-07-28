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

package encodingregistry

import (
	"io"
	"testing"

	"cuelang.org/go/cue/build"
	"cuelang.org/go/internal/filetypes"
)

var benchmarkDecoder = func(*build.File, io.Reader) (Decoder, error) {
	return nil, nil
}

var benchmarkEncoding = Encoding{
	Name:       "benchmark",
	Extensions: []string{".benchmark"},
	Info:       FileInfo{Form: "data"},
	NewDecoder: benchmarkDecoder,
}

// prewarmRegistration materializes the generated built-in maps through the
// same path as a real registration, then resets only the dynamic registry.
// sync.OnceValue deliberately provides no reset: first-use map construction is
// measured separately by internal/filetypes.BenchmarkBuiltinDataMaterialization.
func prewarmRegistration(b *testing.B) {
	b.Helper()
	filetypes.ResetDynamicRegistryForTesting()
	if err := registerWithoutFullValidation(benchmarkEncoding); err != nil {
		b.Fatal(err)
	}
	filetypes.ResetDynamicRegistryForTesting()
}

// BenchmarkRegisterWithoutFullValidationWarm measures evaluator-free construction and
// publication after generated built-in data is materialized. Resetting the
// process-global dynamic registry is required between iterations by its
// add-only contract and is deliberately outside the timer.
func BenchmarkRegisterWithoutFullValidationWarm(b *testing.B) {
	prewarmRegistration(b)
	b.ReportAllocs()
	b.Cleanup(filetypes.ResetDynamicRegistryForTesting)
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		filetypes.ResetDynamicRegistryForTesting()
		b.StartTimer()
		if err := registerWithoutFullValidation(benchmarkEncoding); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegisterWarm measures the normal registration path after the
// process-wide CUE template has been compiled. Registry reset remains outside
// the timer, matching BenchmarkRegisterWithoutFullValidationWarm.
func BenchmarkRegisterWarm(b *testing.B) {
	if _, err := templateValue(); err != nil {
		b.Fatal(err)
	}
	prewarmRegistration(b)
	b.ReportAllocs()
	b.Cleanup(filetypes.ResetDynamicRegistryForTesting)
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		filetypes.ResetDynamicRegistryForTesting()
		b.StartTimer()
		if err := register(benchmarkEncoding); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateWarm measures normal validation once the process-wide CUE
// template has already been compiled.
func BenchmarkValidateWarm(b *testing.B) {
	if err := validateDeclaration(benchmarkEncoding); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := validateDeclaration(benchmarkEncoding); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompileTemplateCold isolates the one-time CUE template compilation
// that RegisterWithoutFullValidation avoids.
func BenchmarkCompileTemplateCold(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := compileTemplate(); err != nil {
			b.Fatal(err)
		}
	}
}
