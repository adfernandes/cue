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

package encodingregistry_test

import (
	"strings"
	"testing"

	"cuelang.org/go/unstable/encodingregistry"
)

// TestEveryAspectConstantIsMapped enforces that every exported Aspect
// constant names an aspect the registration path knows: a declaration
// using it must never be rejected as an unknown aspect or an unknown
// template field. A constant may still conflict with a particular
// form's fixed value, so the test accepts either polarity before
// judging. Together with the aspect-name/template test in
// internal/filetypes, this keeps the constants, the aspect-name
// table, and the types.cue #FileInfo template in one-to-one
// correspondence.
func TestEveryAspectConstantIsMapped(t *testing.T) {
	aspects := []encodingregistry.Aspect{
		encodingregistry.AspectAttributes,
		encodingregistry.AspectConstraints,
		encodingregistry.AspectCycles,
		encodingregistry.AspectData,
		encodingregistry.AspectDefinitions,
		encodingregistry.AspectDocs,
		encodingregistry.AspectIncomplete,
		encodingregistry.AspectImports,
		encodingregistry.AspectKeepDefaults,
		encodingregistry.AspectOptional,
		encodingregistry.AspectReferences,
		encodingregistry.AspectStream,
	}
	for _, a := range aspects {
		t.Run(string(a), func(t *testing.T) {
			var lastErr error
			for _, v := range []bool{true, false} {
				reset(t)
				lastErr = encodingregistry.Register(encodingregistry.Encoding{
					Name:       "zzaspect",
					Info:       encodingregistry.FileInfo{Aspects: map[encodingregistry.Aspect]bool{a: v}},
					NewDecoder: newKVDecoder,
				})
				if lastErr == nil {
					return
				}
				if msg := lastErr.Error(); strings.Contains(msg, "unknown aspect") ||
					strings.Contains(msg, "field not allowed") {
					t.Fatalf("aspect %q is not mapped: %v", a, lastErr)
				}
			}
			t.Fatalf("aspect %q rejected with both values: %v", a, lastErr)
		})
	}
}
