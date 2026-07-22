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

package filetypes_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"cuelang.org/go/cue/build"
	"cuelang.org/go/internal/filetypes"
)

// TestPBQualifierResolvesByExtension pins the resolution of the pb
// qualifier, whose tag value is a struct-level disjunction: the file
// extension has to participate in choosing the disjunct, so that
// "pb: x.json" selects the protobuf-JSON branch rather than the
// binarypb default. Resolving the extension after the disjunction was
// already discharged made every extension resolve to binarypb.
func TestPBQualifierResolvesByExtension(t *testing.T) {
	testCases := []struct {
		filename string
		enc      build.Encoding
		interp   build.Interpretation
	}{
		{"x.json", build.JSON, build.ProtobufJSON},
		{"x.yaml", build.YAML, build.ProtobufJSON},
		{"x.proto", build.Protobuf, build.ProtobufJSON},
		{"x.textproto", build.TextProto, build.ProtobufJSON},
	}
	for _, mode := range []filetypes.Mode{filetypes.Input, filetypes.Export, filetypes.Def, filetypes.Eval} {
		for _, tc := range testCases {
			f, err := filetypes.ParseFileAndType(tc.filename, "pb", mode)
			if err != nil {
				t.Errorf("[%s] pb: %s: unexpected error: %v", mode, tc.filename, err)
				continue
			}
			if f.Encoding != tc.enc || f.Interpretation != tc.interp {
				t.Errorf("[%s] pb: %s: got encoding %q interpretation %q; want %q, %q",
					mode, tc.filename, f.Encoding, f.Interpretation, tc.enc, tc.interp)
			}
		}
		// An unknown extension still reports the extension as the
		// cause rather than silently resolving to the default branch.
		if _, err := filetypes.ParseFileAndType("x.unknown", "pb", mode); err == nil {
			t.Errorf("[%s] pb: x.unknown: got nil error; want unknown file extension", mode)
		} else if !strings.Contains(err.Error(), "unknown file extension") {
			t.Errorf("[%s] pb: x.unknown: got %v; want unknown file extension", mode, err)
		}
	}
}

// TestEncodingTagPinsEncoding is the counterpart to the pb case: a tag
// that declares the encoding as a regular field pins it, and the file
// extension is then ignored rather than conflicting with it.
func TestEncodingTagPinsEncoding(t *testing.T) {
	f, err := filetypes.ParseFileAndType("x.txt", "json", filetypes.Input)
	if err != nil {
		t.Fatalf("json: x.txt: unexpected error: %v", err)
	}
	if f.Encoding != build.JSON {
		t.Errorf("json: x.txt: got encoding %q; want json", f.Encoding)
	}
	// Letting the extension participate unconditionally would leak the
	// extension's auto interpretation into an explicitly tagged file.
	f, err = filetypes.ParseFileAndType("x.json", "json", filetypes.Input)
	if err != nil {
		t.Fatalf("json: x.json: unexpected error: %v", err)
	}
	if f.Interpretation != "" {
		t.Errorf("json: x.json: got interpretation %q; want none", f.Interpretation)
	}
}

// TestFromFileRejectsUnknownNames pins that FromFile validates every
// name it is given. Previously a non-empty Form suppressed the
// interpretation check and a non-empty Interpretation suppressed the
// encoding check, so with both set neither name was validated and the
// bogus value was echoed back in the returned FileInfo.
func TestFromFileRejectsUnknownNames(t *testing.T) {
	testCases := []struct {
		name string
		file *build.File
		want string
	}{{
		name: "unknown encoding with interpretation and form",
		file: &build.File{Filename: "x", Encoding: "bogus", Interpretation: "jsonschema", Form: "data"},
		want: `unknown encoding "bogus"`,
	}, {
		name: "unknown encoding with interpretation",
		file: &build.File{Filename: "x", Encoding: "bogus", Interpretation: "jsonschema"},
		want: `unknown encoding "bogus"`,
	}, {
		name: "unknown interpretation with form",
		file: &build.File{Filename: "x", Encoding: "json", Interpretation: "bogus", Form: "data"},
		want: `unknown interpretation "bogus"`,
	}, {
		name: "unknown form",
		file: &build.File{Filename: "x", Encoding: "json", Form: "bogus"},
		want: `unknown form "bogus"`,
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []filetypes.Mode{filetypes.Input, filetypes.Export, filetypes.Def, filetypes.Eval} {
				fi, err := filetypes.FromFile(tc.file, mode)
				if err == nil {
					t.Errorf("[%s] got nil error and %#v; want %s", mode, fi, tc.want)
					continue
				}
				if !strings.Contains(err.Error(), tc.want) {
					t.Errorf("[%s] got %v; want %s", mode, err, tc.want)
				}
			}
		})
	}
}

// TestFromFileFormConflictError pins the diagnosis of a form that the
// mode's encoding does not admit. Every such miss used to report
// "no encoding specified", which is misleading when the caller did
// supply an encoding.
func TestFromFileFormConflictError(t *testing.T) {
	// modes.eval.encodings.cue is forms.final, which admits no other
	// form; the CUE evaluator reports the same conflict.
	f := &build.File{Filename: "x.cue", Encoding: build.CUE, Form: "data"}
	_, err := filetypes.FromFile(f, filetypes.Eval)
	if err == nil {
		t.Fatalf("got nil error; want a form conflict")
	}
	if got := err.Error(); !strings.Contains(got, `form "data"`) ||
		!strings.Contains(got, "not supported") ||
		!strings.Contains(got, `encoding "cue"`) {
		t.Errorf("got %q; want a message naming the form, the encoding, and the mode", got)
	}
	if strings.Contains(err.Error(), "no encoding specified") {
		t.Errorf("got %q; want the form conflict, not the missing-encoding error", err)
	}
}

// TestFormQualifierOutputMatrix pins the adjudicated behavior of the
// form tags used as output qualifiers. types.cue makes most of these
// combinations contradictory (modes.eval.encodings.cue is forms.final,
// and so on); the pre-driver lookup tables accepted them only because
// the table generator dropped form-only tags in every mode after
// input. The three combinations that types.cue does admit must keep
// working.
func TestFormQualifierOutputMatrix(t *testing.T) {
	ok := map[string]bool{
		"export/data":  true,
		"def/graph":    true,
		"def/schema":   true,
		"export/graph": false, "export/dag": false, "export/schema": false,
		"eval/data": false, "eval/graph": false, "eval/dag": false, "eval/schema": false,
		"def/data": false, "def/dag": false,
	}
	modes := map[string]filetypes.Mode{
		"export": filetypes.Export,
		"eval":   filetypes.Eval,
		"def":    filetypes.Def,
	}
	for _, modeName := range []string{"export", "eval", "def"} {
		for _, form := range []string{"data", "graph", "dag", "schema"} {
			key := modeName + "/" + form
			mode := modes[modeName]
			f, err := filetypes.ParseFileAndType("out.cue", form, mode)
			if err != nil {
				t.Errorf("%s: ParseFileAndType: unexpected error: %v", key, err)
				continue
			}
			// The form tag is reported, which the old tables lost.
			if string(f.Form) != form {
				t.Errorf("%s: got form %q; want %q", key, f.Form, form)
			}
			_, err = filetypes.FromFile(f, mode)
			if got := err == nil; got != ok[key] {
				t.Errorf("%s: FromFile ok=%v (%v); want ok=%v", key, got, err, ok[key])
			}
		}
	}
}

// TestFormTagWithDataOnlyExtension pins that a form tag combined with
// an extension whose encoding fixes form "data" is a conflict, as the
// CUE evaluator reports. The old tables accepted these in every mode
// but input, an inconsistency caused by the same generator defect.
func TestFormTagWithDataOnlyExtension(t *testing.T) {
	for _, mode := range []filetypes.Mode{filetypes.Export, filetypes.Def, filetypes.Eval} {
		for _, form := range []string{"schema", "graph", "dag"} {
			for _, filename := range []string{"x.txt", "x.wasm"} {
				if _, err := filetypes.ParseFileAndType(filename, form, mode); err == nil {
					t.Errorf("[%s] %s: %s: got nil error; want a conflict", mode, form, filename)
				}
			}
		}
	}
}

// TestInvalidModeErrors pins that an out-of-range Mode is reported as
// an error rather than panicking with an index-out-of-range on the
// per-mode registry arrays. The earlier lookup tables degraded to a
// lookup miss for such modes.
func TestInvalidModeErrors(t *testing.T) {
	for _, mode := range []filetypes.Mode{filetypes.NumModes, filetypes.Mode(99), filetypes.Mode(-1)} {
		if _, err := filetypes.ParseFile("x.json", mode); err == nil {
			t.Errorf("ParseFile with mode %d: got nil error; want invalid mode", int(mode))
		} else if !strings.Contains(err.Error(), "invalid mode") {
			t.Errorf("ParseFile with mode %d: got %v; want invalid mode", int(mode), err)
		}
		f := &build.File{Filename: "x", Encoding: build.JSON}
		if _, err := filetypes.FromFile(f, mode); err == nil {
			t.Errorf("FromFile with mode %d: got nil error; want invalid mode", int(mode))
		} else if !strings.Contains(err.Error(), "invalid mode") {
			t.Errorf("FromFile with mode %d: got %v; want invalid mode", int(mode), err)
		}
	}
}

// TestSubsidiaryBoolTagDifferingDefaults pins CUE's default semantics
// for Boolean subsidiary tags. Registering a tag name that a built-in
// interpretation also declares, with a different default, is legal:
// the two defaults cancel and the tag is simply reported without a
// value, exactly as (*false | bool) & (*true | bool) is bool. Treating
// the differing defaults as a conflict rejected the combination
// outright, and no explicit value could rescue it.
func TestSubsidiaryBoolTagDifferingDefaults(t *testing.T) {
	filetypes.ResetDynamicRegistryForTesting()
	t.Cleanup(filetypes.ResetDynamicRegistryForTesting)

	strictTrue := true
	err := filetypes.RegisterEncoding(filetypes.DynamicEncoding{
		Name:       "zzbool",
		Extensions: []string{".zzbool"},
		Info:       filetypes.DynamicFileInfo{Form: "schema"},
		BoolTags:   map[string]*bool{"strict": &strictTrue},
		Codec: filetypes.Codec{
			NewDecoder: func(*build.File, io.Reader) (filetypes.Decoder, error) {
				return nil, errors.New("zzbool decoding is not exercised by this test")
			},
		},
	})
	if err != nil {
		t.Fatalf("RegisterEncoding: %v", err)
	}

	// The combination resolves; strict has no default left, so it is
	// not reported at all.
	f, err := filetypes.ParseFileAndType("x.zzbool", "jsonschema+zzbool", filetypes.Input)
	if err != nil {
		t.Fatalf("jsonschema+zzbool: unexpected error: %v", err)
	}
	if _, present := f.BoolTags["strict"]; present {
		t.Errorf("jsonschema+zzbool: strict = %v; want absent (defaults canceled)", f.BoolTags["strict"])
	}

	// An explicit value is still honored.
	f, err = filetypes.ParseFileAndType("x.zzbool", "jsonschema+zzbool+strict", filetypes.Input)
	if err != nil {
		t.Fatalf("jsonschema+zzbool+strict: unexpected error: %v", err)
	}
	if !f.BoolTags["strict"] {
		t.Errorf("jsonschema+zzbool+strict: strict = %v; want true", f.BoolTags["strict"])
	}
}
