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

// TestDisjunctionDomainParity pins the fix for the open-data driver
// accepting encoding/form/interpretation combinations that types.cue
// (and the earlier generated tables) reject. The driver used to record
// only a field's disjunction default and discard its allowed
// alternatives, so any concrete value could override the default
// unchecked. Each combination below must resolve to an error because
// the requested form is not a member of the field's disjunction domain.
func TestDisjunctionDomainParity(t *testing.T) {
	t.Run("qualifier", func(t *testing.T) {
		cases := []struct {
			file, scope string
		}{
			{"x.json", "jsonschema+data"},
			{"x.json", "openapi+dag"},
			{"x.json", "auto+data"},
		}
		for _, tc := range cases {
			if f, err := ParseFileAndType(tc.file, tc.scope, Input); err == nil {
				t.Errorf("ParseFileAndType(%q, %q): got %+v, want error (form not in domain)", tc.file, tc.scope, f)
			}
		}
	})

	t.Run("fromfile", func(t *testing.T) {
		cases := []*build.File{
			{Filename: "x", Encoding: "proto", Form: "dag"},
			{Filename: "x", Encoding: "code", Form: "data"},
		}
		for _, b := range cases {
			if fi, err := FromFile(b, Input); err == nil {
				t.Errorf("FromFile(enc=%s, form=%s): got %+v, want error (form not in domain)", b.Encoding, b.Form, fi)
			}
		}
	})
}
