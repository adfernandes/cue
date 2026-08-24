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
	"errors"
	"io"
	"strings"
	"testing"

	"cuelang.org/go/cue/build"
)

func TestValidateEncodingComposition(t *testing.T) {
	tests := []struct {
		name string
		info DynamicFileInfo
	}{
		{
			name: "interpretation-form",
			info: DynamicFileInfo{Interpretation: "jsonschema", Form: "data"},
		},
		{
			name: "interpretation-aspect",
			info: DynamicFileInfo{
				Interpretation: "pb",
				Form:           "data",
				Aspects:        map[string]bool{"stream": false},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := DynamicEncoding{Name: "bad", Info: test.info}
			err := ValidateEncoding(e)
			if err == nil || !strings.Contains(err.Error(), "interpretation") {
				t.Fatalf("ValidateEncoding() error = %v; want interpretation composition error", err)
			}

			ResetDynamicRegistryForTesting()
			t.Cleanup(ResetDynamicRegistryForTesting)
			err = RegisterEncoding(e)
			if err == nil || !strings.Contains(err.Error(), "interpretation") {
				t.Fatalf("RegisterEncoding() error = %v; want interpretation composition error", err)
			}
			// A failed registration must publish no partial state; the same
			// name remains available for a corrected declaration.
			e.Info = DynamicFileInfo{Form: "data"}
			if err := RegisterEncoding(e); err != nil {
				t.Fatalf("corrected RegisterEncoding() failed after rejection: %v", err)
			}
		})
	}
}

// TestValidateEncodingBuiltinConflicts checks that ValidateEncoding
// reports conflicts with built-in names and extensions. The built-in
// file types are compile-time constants, so a build-time validation
// (encodingregistry.Validate) can catch these conflicts; conflicts
// with prior runtime registrations remain RegisterEncoding's alone,
// keeping ValidateEncoding state-independent and side-effect-free.
func TestValidateEncodingBuiltinConflicts(t *testing.T) {
	// A built-in encoding name.
	err := ValidateEncoding(DynamicEncoding{Name: "json"})
	conflict, ok := errors.AsType[*ConflictError](err)
	if !ok {
		t.Fatalf("ValidateEncoding(name json) = %v; want *ConflictError", err)
	}
	if !conflict.BuiltIn || conflict.Kind != "name" || conflict.Owner != "json" {
		t.Fatalf("ValidateEncoding(name json) = %#v; want built-in name conflict owned by json", conflict)
	}

	// A built-in extension.
	err = ValidateEncoding(DynamicEncoding{Name: "kvext", Extensions: []string{".json"}})
	if conflict, ok = errors.AsType[*ConflictError](err); !ok {
		t.Fatalf("ValidateEncoding(extension .json) = %v; want *ConflictError", err)
	}
	if !conflict.BuiltIn || conflict.Kind != "extension" || conflict.Owner != "json" {
		t.Fatalf("ValidateEncoding(extension .json) = %#v; want built-in extension conflict owned by json", conflict)
	}

	// A built-in tag that is not an encoding renders as a tag
	// collision, not as a nonexistent encoding.
	err = ValidateEncoding(DynamicEncoding{Name: "strict"})
	if _, ok := errors.AsType[*ConflictError](err); !ok {
		t.Fatalf("ValidateEncoding(name strict) = %v; want *ConflictError", err)
	}
	if want := `cannot register encoding "strict": name already registered as a built-in tag`; err.Error() != want {
		t.Errorf("ValidateEncoding(name strict) = %q; want %q", err, want)
	}

	// ValidateEncoding does not consult dynamic registrations: a name
	// taken at runtime still validates, while registering it again
	// conflicts.
	ResetDynamicRegistryForTesting()
	t.Cleanup(ResetDynamicRegistryForTesting)
	if err := RegisterEncoding(DynamicEncoding{Name: "vfree"}); err != nil {
		t.Fatalf("RegisterEncoding(vfree): %v", err)
	}
	if err := ValidateEncoding(DynamicEncoding{Name: "vfree"}); err != nil {
		t.Errorf("ValidateEncoding must not consult dynamic registrations; got %v", err)
	}
	if err := RegisterEncoding(DynamicEncoding{Name: "vfree"}); err == nil {
		t.Error("RegisterEncoding(vfree) twice unexpectedly succeeded")
	}
}

// TestRegisterEncodingRequiresDecoder checks the codec invariant: a
// codec that declares any machinery (an encoder or binary handling)
// must include a decoder — the syntax-plane NewDecoder or a
// value-plane Codec.Value. A fully zero Codec remains permitted for
// the resolution-only registrations in-package tests use.
func TestRegisterEncodingRequiresDecoder(t *testing.T) {
	dec := func(*build.File, io.Reader) (Decoder, error) { return nil, nil }
	enc := func(*build.File, io.Writer) (Encoder, error) { return nil, nil }
	tests := []struct {
		name    string
		codec   Codec
		wantErr bool
	}{
		{name: "zero codec is resolution-only", codec: Codec{}, wantErr: false},
		{name: "encoder without decoder", codec: Codec{NewEncoder: enc}, wantErr: true},
		{name: "binary without decoder", codec: Codec{Binary: true}, wantErr: true},
		{name: "syntax decoder", codec: Codec{NewDecoder: dec}, wantErr: false},
		{name: "value-plane codec", codec: Codec{Value: &struct{}{}}, wantErr: false},
		{name: "encoder with syntax decoder", codec: Codec{NewDecoder: dec, NewEncoder: enc}, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := DynamicEncoding{Name: "codeccheck", Codec: test.codec}
			checkErr := func(call string, err error) {
				t.Helper()
				if test.wantErr {
					if err == nil || !strings.Contains(err.Error(), "decoder is required") {
						t.Fatalf("%s = %v; want decoder-required error", call, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("%s = %v; want success", call, err)
				}
			}
			checkErr("ValidateEncoding()", ValidateEncoding(e))

			ResetDynamicRegistryForTesting()
			t.Cleanup(ResetDynamicRegistryForTesting)
			checkErr("RegisterEncoding()", RegisterEncoding(e))
		})
	}
}

// TestPerModeOverrideReplacesBase pins the replacement semantics of
// per-mode overlays: the overlay's form replaces the base form for its
// mode rather than unifying with it — forms.schema & forms.data would
// be an unsatisfiable conflict — mirroring how built-in encodings vary
// form per mode in types.cue. Wire-format registrations (a schema
// encoding that exports as data) depend on this.
func TestPerModeOverrideReplacesBase(t *testing.T) {
	ResetDynamicRegistryForTesting()
	t.Cleanup(ResetDynamicRegistryForTesting)

	err := RegisterEncoding(DynamicEncoding{
		Name: "permode",
		Info: DynamicFileInfo{Form: "schema"},
		PerMode: map[Mode]DynamicFileInfo{
			Export: {Form: "data"},
		},
	})
	if err != nil {
		t.Fatalf("RegisterEncoding: %v", err)
	}
	fi, err := FromFile(&build.File{Encoding: "permode"}, Input)
	if err != nil {
		t.Fatalf("FromFile(Input): %v", err)
	}
	if fi.Form != build.Schema {
		t.Errorf("Input mode Form = %q; want %q", fi.Form, build.Schema)
	}
	fi, err = FromFile(&build.File{Encoding: "permode"}, Export)
	if err != nil {
		t.Fatalf("FromFile(Export): %v", err)
	}
	if fi.Form != build.Data {
		t.Errorf("Export mode Form = %q; want %q", fi.Form, build.Data)
	}
}

func TestDynamicFormRefinement(t *testing.T) {
	tests := []struct {
		name        string
		declared    string
		wantDefault build.Form
		allowed     []build.Form
		rejected    []build.Form
	}{
		{
			name:        "schema",
			declared:    "schema",
			wantDefault: build.Schema,
			allowed:     []build.Form{build.Final, build.Graph},
			rejected:    []build.Form{build.DAG, build.Data},
		},
		{
			name:        "graph",
			declared:    "graph",
			wantDefault: build.Graph,
			allowed:     []build.Form{build.DAG, build.Data},
			rejected:    []build.Form{build.Schema, build.Final},
		},
		{
			name:        "dag",
			declared:    "dag",
			wantDefault: build.DAG,
			allowed:     []build.Form{build.Data},
			rejected:    []build.Form{build.Graph, build.Schema, build.Final},
		},
		{
			name:        "data",
			declared:    "data",
			wantDefault: build.Data,
			rejected:    []build.Form{build.DAG, build.Graph, build.Schema, build.Final},
		},
		{
			name:        "final",
			declared:    "final",
			wantDefault: build.Final,
			rejected:    []build.Form{build.Data, build.DAG, build.Graph, build.Schema},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ResetDynamicRegistryForTesting()
			t.Cleanup(ResetDynamicRegistryForTesting)
			e := DynamicEncoding{Name: "dynamic" + test.name, Info: DynamicFileInfo{Form: test.declared}}
			if err := RegisterEncoding(e); err != nil {
				t.Fatal(err)
			}
			checkForm := func(form build.Form, wantOK bool) {
				t.Helper()
				fi, err := FromFile(&build.File{Encoding: build.Encoding(e.Name), Form: form}, Input)
				if !wantOK {
					if err == nil {
						t.Fatalf("FromFile(form %q) unexpectedly succeeded: %#v", form, fi)
					}
					return
				}
				if err != nil {
					t.Fatalf("FromFile(form %q): %v", form, err)
				}
				want := form
				if want == "" {
					want = test.wantDefault
				}
				if fi.Form != want {
					t.Fatalf("FromFile(form %q).Form = %q; want %q", form, fi.Form, want)
				}
			}
			checkForm("", true)
			for _, form := range test.allowed {
				checkForm(form, true)
			}
			for _, form := range test.rejected {
				checkForm(form, false)
			}
		})
	}
}
