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
	"os"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"cuelang.org/go/cue/build"
	"cuelang.org/go/internal/filetypes"
)

func readTypesCUE() ([]byte, error) {
	return os.ReadFile("../../types.cue")
}

var loadRegistry = sync.OnceValues(func() (*Registry, error) {
	src, err := readTypesCUE()
	if err != nil {
		return nil, err
	}
	return Load(src)
})

func TestAspectNames(t *testing.T) {
	want := [numAspects]string{
		"attributes",
		"constraints",
		"cycles",
		"data",
		"definitions",
		"docs",
		"incomplete",
		"imports",
		"keepDefaults",
		"optional",
		"references",
		"stream",
	}
	if diff := cmp.Diff(want, aspectNames); diff != "" {
		t.Fatalf("aspectNames mismatch (-want +got):\n%s", diff)
	}
}

// TestParityToFile runs the prototype against the shipping
// implementation on the operations the T006 benchmark exercises plus a
// representative sample of qualifier and mode combinations, comparing
// outputs field by field (T011). Divergences are test failures here and
// are recorded in research.md §8.3.
func TestParityToFile(t *testing.T) {
	r, err := loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		file  string
		scope string
		mode  filetypes.Mode
	}{
		{"foo.json", "", filetypes.Input},
		{"foo.json", "", filetypes.Export},
		{"foo.json", "", filetypes.Def},
		{"foo.json", "", filetypes.Eval},
		{"bar.data", "json+schema", filetypes.Input},
		{"x.yaml", "", filetypes.Input},
		{"x.yml", "", filetypes.Def},
		{"a.cue", "", filetypes.Input},
		{"a.cue", "", filetypes.Export},
		{"f.toml", "", filetypes.Input},
		{"f.txt", "", filetypes.Input},
		{"f.proto", "", filetypes.Input},
		{"f.textproto", "", filetypes.Input},
		{"-", "", filetypes.Input},
		{"-", "", filetypes.Export},
		{"schema.json", "jsonschema", filetypes.Input},
		{"api.json", "openapi", filetypes.Input},
		{"d.yaml", "graph", filetypes.Input},
		{"d.yaml", "dag", filetypes.Input},
		{"d.yaml", "data", filetypes.Input},
		{"out.cue", "cue+compact", filetypes.Export},
		{"y.yaml", "yaml+indentSequences=false", filetypes.Export},
		{"x.xml", "xml+koala", filetypes.Input},
		{"g.go", "go", filetypes.Input},
		{"f.bin", "binary", filetypes.Input},
		{"unknown.bar", "", filetypes.Input},
		{"noext", "", filetypes.Input},
		{"f.json", "badtag", filetypes.Input},
		{"f.pb", "pb", filetypes.Input},
	}
	for _, c := range cases {
		name := c.scope + ":" + c.file + "/" + c.mode.String()
		t.Run(name, func(t *testing.T) {
			want, wantErr := filetypes.ParseFileAndType(c.file, c.scope, c.mode)
			got, gotErr := r.ParseFileAndType(c.file, c.scope, Mode(c.mode))
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("error divergence: shipping=%v prototype=%v", wantErr, gotErr)
			}
			if wantErr != nil {
				return // both error; text parity is an implementation task
			}
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("result divergence -shipping +prototype:\n%s", diff)
			}
		})
	}
}

// TestParityFromFile compares FromFile outputs (T011).
func TestParityFromFile(t *testing.T) {
	r, err := loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   build.File
		mode filetypes.Mode
	}{
		{build.File{Filename: "x.yaml", Encoding: build.YAML}, filetypes.Input},
		{build.File{Filename: "x.yaml", Encoding: build.YAML}, filetypes.Def},
		{build.File{Filename: "x.json", Encoding: build.JSON}, filetypes.Input},
		{build.File{Filename: "x.json", Encoding: build.JSON, Interpretation: build.JSONSchema}, filetypes.Input},
		{build.File{Filename: "x.json", Encoding: build.JSON, Form: build.Schema}, filetypes.Input},
		{build.File{Filename: "x.cue", Encoding: build.CUE}, filetypes.Def},
		{build.File{Filename: "x.cue", Encoding: build.CUE, Form: build.Data}, filetypes.Input},
		{build.File{Filename: "x.toml", Encoding: build.TOML}, filetypes.Input},
		{build.File{Filename: "x.txt", Encoding: build.Text}, filetypes.Export},
		{build.File{}, filetypes.Input},
	}
	for _, c := range cases {
		name := string(c.in.Encoding) + "/" + string(c.in.Interpretation) + "/" + string(c.in.Form) + "/" + c.mode.String()
		t.Run(name, func(t *testing.T) {
			want, wantErr := filetypes.FromFile(&c.in, c.mode)
			got, gotErr := r.FromFile(&c.in, Mode(c.mode))
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("error divergence: shipping=%v prototype=%v", wantErr, gotErr)
			}
			if wantErr != nil {
				return
			}
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("result divergence -shipping +prototype:\n%s", diff)
			}
		})
	}
}

// TestRegisterDynamic exercises the C4 registration path end to end at
// the resolution level: an encoding unknown to types.cue becomes
// resolvable by extension and qualifier, and conflicts are refused.
func TestRegisterDynamic(t *testing.T) {
	src, err := os.ReadFile("../../types.cue")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	decl := []byte(`
encoding: "capnp"
form:     "schema"
stream:   false
`)
	if err := r.Register("capnp", []string{".capnp"}, decl); err != nil {
		t.Fatal(err)
	}
	f, err := r.ParseFileAndType("x.capnp", "", Input)
	if err != nil {
		t.Fatal(err)
	}
	if f.Encoding != "capnp" || f.Form != build.Schema {
		t.Errorf("unexpected resolution: %+v", f)
	}
	f, err = r.ParseFileAndType("otherfile", "capnp", Input)
	if err != nil {
		t.Fatal(err)
	}
	if f.Encoding != "capnp" {
		t.Errorf("qualifier resolution failed: %+v", f)
	}
	// Add-only conflicts (D-002).
	if err := r.Register("capnp", nil, decl); err == nil {
		t.Error("re-registration of capnp not refused")
	}
	if err := r.Register("json", nil, decl); err == nil {
		t.Error("collision with built-in name not refused")
	}
	if err := r.Register("capnp2", []string{".json"}, decl); err == nil {
		t.Error("collision with built-in extension not refused")
	}
	// Template validation (C4): a declaration violating #FileInfo.
	bad := []byte(`
encoding: "badenc"
form:     42
`)
	if err := r.Register("badenc", nil, bad); err == nil {
		t.Error("non-conforming registration not refused")
	}
}
