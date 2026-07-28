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
	"bytes"
	"io"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/internal/encoding"
	"cuelang.org/go/internal/filetypes"
	"cuelang.org/go/unstable/encodingregistry"
	"github.com/go-quicktest/qt"
)

// A stub value-plane codec that serializes a cue.Value as CUE text,
// preserving the value lattice (constraints, disjunctions, definitions).
// A syntax-plane codec forced to concrete syntax could not carry these.

func newVPEncoder(f *build.File, w io.Writer) (encodingregistry.ValueEncoder, error) {
	return &vpEncoder{w: w}, nil
}

type vpEncoder struct{ w io.Writer }

func (e *vpEncoder) Encode(v cue.Value) error {
	b, err := format.Node(v.Syntax(
		cue.Optional(true),
		cue.Definitions(true),
		cue.Hidden(true),
		cue.ErrorsAsValues(true),
	))
	if err != nil {
		return err
	}
	_, err = e.w.Write(b)
	return err
}

// TestValuePlaneDirectValue verifies both sides of the decoder contract: the
// exact cue.Value remains available to value-aware consumers, while File keeps
// a compilable compatibility projection, including its import preamble.
func TestValuePlaneDirectValue(t *testing.T) {
	reset(t)
	err := encodingregistry.Register(encodingregistry.Encoding{
		Name:            "vpdirect",
		Extensions:      []string{".vpdirect"},
		Info:            encodingregistry.FileInfo{Form: "schema"},
		NewValueDecoder: newVPDecoder,
		NewValueEncoder: newVPEncoder,
	})
	qt.Assert(t, qt.IsNil(err))

	ctx := cuecontext.New()
	src := []byte(`
import "strings"

base: strings.MinRunes(1)
alias: base
`)
	dec := encoding.NewDecoder(ctx, &build.File{
		Filename:       "in.vpdirect",
		Encoding:       "vpdirect",
		Form:           "schema",
		Interpretation: build.Auto,
		Source:         src,
	}, &encoding.Config{Mode: filetypes.Input})
	qt.Assert(t, qt.IsNil(dec.Err()))

	got, ok := dec.DecodedValue()
	qt.Assert(t, qt.IsTrue(ok))
	qt.Assert(t, qt.IsNil(got.Err()))
	root, ref := got.LookupPath(cue.ParsePath("alias")).ReferencePath()
	qt.Assert(t, qt.IsTrue(root.Exists()))
	qt.Assert(t, qt.Equals(ref.String(), "base"))

	// The syntax fallback must retain the import declaration rather than
	// turning only the post-preamble declarations into a struct expression.
	f := dec.File()
	qt.Assert(t, qt.IsNotNil(dec.SourceExpr()))
	imports := 0
	for range f.ImportSpecs() {
		imports++
	}
	qt.Assert(t, qt.Equals(imports, 1))
	rebuilt := ctx.BuildFile(f)
	qt.Assert(t, qt.IsNil(rebuilt.Err()))
}

// TestValuePlaneInterpretationPreservesSource verifies that an explicit
// interpretation can replace File without discarding the value-plane source
// expression used by --with-context and path placement.
func TestValuePlaneInterpretationPreservesSource(t *testing.T) {
	reset(t)
	err := encodingregistry.Register(encodingregistry.Encoding{
		Name:            "vpjsonschema",
		Extensions:      []string{".vpjsonschema"},
		Info:            encodingregistry.FileInfo{Form: "schema"},
		NewValueDecoder: newVPDecoder,
		NewValueEncoder: newVPEncoder,
	})
	qt.Assert(t, qt.IsNil(err))

	ctx := cuecontext.New()
	dec := encoding.NewDecoder(ctx, &build.File{
		Filename:       "in.vpjsonschema",
		Encoding:       "vpjsonschema",
		Form:           "schema",
		Interpretation: build.JSONSchema,
		Source: []byte(`{
			"$schema": "http://json-schema.org/draft-07/schema",
			"$id": "https://example.com/person.schema.json",
			title: "Person",
			type: "object",
		}`),
	}, &encoding.Config{Mode: filetypes.Input})
	qt.Assert(t, qt.IsNil(dec.Err()))

	// Interpretation consumed the direct value and produced a new file.
	_, ok := dec.DecodedValue()
	qt.Assert(t, qt.IsFalse(ok))
	qt.Assert(t, qt.IsNotNil(dec.File()))
	interpreted, err := format.Node(dec.File())
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.StringContains(string(interpreted), "@jsonschema"))

	// Its original source remains available independently of that file.
	source := dec.SourceExpr()
	qt.Assert(t, qt.IsNotNil(source))
	sourceValue := ctx.BuildExpr(source)
	qt.Assert(t, qt.IsNil(sourceValue.Err()))
	title, err := sourceValue.LookupPath(cue.ParsePath("title")).String()
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(title, "Person"))
}

type singleValueDecoder struct {
	v    cue.Value
	done bool
}

func (d *singleValueDecoder) Decode() (cue.Value, error) {
	if d.done {
		return cue.Value{}, io.EOF
	}
	d.done = true
	return d.v, nil
}

// TestValuePlaneBottomRoundTrip verifies that a semantic bottom is payload,
// not a transport error, for an incomplete value-plane encoding. Concrete
// modes continue to reject it through the normal validation path.
func TestValuePlaneBottomRoundTrip(t *testing.T) {
	reset(t)
	err := encodingregistry.Register(encodingregistry.Encoding{
		Name:       "vpbottom",
		Extensions: []string{".vpbottom"},
		Info:       encodingregistry.FileInfo{Form: "schema"},
		PerMode: map[encodingregistry.Mode]encodingregistry.FileInfo{
			encodingregistry.Export: {Form: "data"},
		},
		NewValueDecoder: func(ctx *cue.Context, _ *build.File, _ io.Reader) (encodingregistry.ValueDecoder, error) {
			return &singleValueDecoder{v: ctx.CompileString(`1 & 2`)}, nil
		},
		NewValueEncoder: newVPEncoder,
	})
	qt.Assert(t, qt.IsNil(err))

	ctx := cuecontext.New()
	dec := encoding.NewDecoder(ctx, &build.File{
		Filename:       "in.vpbottom",
		Encoding:       "vpbottom",
		Form:           "schema",
		Interpretation: build.Auto,
		Source:         []byte("ignored"),
	}, &encoding.Config{Mode: filetypes.Input})
	qt.Assert(t, qt.IsNil(dec.Err()))
	bottom, ok := dec.DecodedValue()
	qt.Assert(t, qt.IsTrue(ok))
	qt.Assert(t, qt.IsNotNil(bottom.Err()))
	qt.Assert(t, qt.IsNotNil(dec.File()))

	// Auto detection found no interpretation and retained bottom as payload;
	// an explicit interpretation still rejects that semantic error.
	explicit := encoding.NewDecoder(ctx, &build.File{
		Filename:       "in.vpbottom",
		Encoding:       "vpbottom",
		Form:           "schema",
		Interpretation: build.JSONSchema,
		Source:         []byte("ignored"),
	}, &encoding.Config{Mode: filetypes.Input})
	qt.Assert(t, qt.IsNotNil(explicit.Err()))

	// An incomplete value-plane mode passes bottom to the codec unchanged.
	var buf bytes.Buffer
	enc, err := encoding.NewEncoder(ctx, &build.File{
		Filename: "out.vpbottom",
		Encoding: "vpbottom",
		Form:     "schema",
	}, &encoding.Config{Mode: filetypes.Input, Out: &buf})
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(enc.Encode(bottom)))
	qt.Assert(t, qt.IsNil(enc.Close()))
	qt.Assert(t, qt.StringContains(buf.String(), "_|_"))

	// The same registration resolves to concrete data in export mode.
	enc, err = encoding.NewEncoder(ctx, &build.File{
		Filename: "out.vpbottom",
		Encoding: "vpbottom",
		Form:     "data",
	}, &encoding.Config{Mode: filetypes.Export, Out: io.Discard})
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNotNil(enc.Encode(bottom)))
}

func (e *vpEncoder) Close() error { return nil }

func newVPDecoder(ctx *cue.Context, f *build.File, r io.Reader) (encodingregistry.ValueDecoder, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &vpDecoder{ctx: ctx, b: b}, nil
}

type vpDecoder struct {
	ctx  *cue.Context
	b    []byte
	done bool
}

func (d *vpDecoder) Decode() (cue.Value, error) {
	if d.done {
		return cue.Value{}, io.EOF
	}
	d.done = true
	v := d.ctx.CompileBytes(d.b)
	return v, v.Err()
}

// TestValuePlaneFidelity registers a value-plane codec and shows that a
// non-concrete value round-trips through internal/encoding with its
// lattice intact — constraints and a disjunction survive — which the
// syntax-plane path (forced concrete) cannot do.
func TestValuePlaneFidelity(t *testing.T) {
	reset(t)
	err := encodingregistry.Register(encodingregistry.Encoding{
		Name:            "vp",
		Extensions:      []string{".vp"},
		Info:            encodingregistry.FileInfo{Form: "schema"},
		NewValueDecoder: newVPDecoder,
		NewValueEncoder: newVPEncoder,
	})
	qt.Assert(t, qt.IsNil(err))

	ctx := cuecontext.New()
	orig := ctx.CompileString(`{a: >5 & <10, b: "x" | "y", c: int}`)
	qt.Assert(t, qt.IsNil(orig.Err()))

	// Encode in input mode (schema form permits incomplete), so a
	// non-concrete value must encode without error.
	var buf bytes.Buffer
	enc, err := encoding.NewEncoder(ctx, &build.File{Filename: "out.vp", Encoding: "vp", Form: "schema"}, &encoding.Config{Mode: filetypes.Input, Out: &buf})
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(enc.Encode(orig)))
	qt.Assert(t, qt.IsNil(enc.Close()))

	// Decode back through internal/encoding.
	f := &build.File{Filename: "in.vp", Encoding: "vp", Form: "schema", Source: buf.Bytes()}
	dec := encoding.NewDecoder(ctx, f, &encoding.Config{Mode: filetypes.Input})
	qt.Assert(t, qt.IsNil(dec.Err()))
	got := ctx.BuildFile(dec.File())
	qt.Assert(t, qt.IsNil(got.Err()))

	// The reconstructed value is equivalent (mutually subsuming): the
	// bounds and disjunction were preserved, not collapsed to concrete.
	qt.Assert(t, qt.IsNil(orig.Subsume(got, cue.Raw())))
	qt.Assert(t, qt.IsNil(got.Subsume(orig, cue.Raw())))
}

// TestValuePlaneConcreteness checks the mode-driven concreteness of the
// value-plane encoder: a non-concrete value encodes in a mode that
// permits incomplete data (export uses form "data" here only as a
// contrast) and is rejected in a concrete mode.
func TestValuePlaneConcreteness(t *testing.T) {
	reset(t)
	err := encodingregistry.Register(encodingregistry.Encoding{
		Name:       "vp2",
		Extensions: []string{".vp2"},
		Info:       encodingregistry.FileInfo{Form: "schema"},
		PerMode: map[encodingregistry.Mode]encodingregistry.FileInfo{
			encodingregistry.Export: {Form: "data"},
		},
		NewValueDecoder: newVPDecoder,
		NewValueEncoder: newVPEncoder,
	})
	qt.Assert(t, qt.IsNil(err))

	ctx := cuecontext.New()
	nonConcrete := ctx.CompileString(`{a: int}`)
	qt.Assert(t, qt.IsNil(nonConcrete.Err()))

	// Input mode: schema form permits incomplete -> non-concrete encodes.
	var buf bytes.Buffer
	enc, err := encoding.NewEncoder(ctx, &build.File{Filename: "o.vp2", Encoding: "vp2", Form: "schema"}, &encoding.Config{Mode: filetypes.Input, Out: &buf})
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(enc.Encode(nonConcrete)))

	// Export mode: form data requires concrete -> non-concrete is rejected.
	var buf2 bytes.Buffer
	enc2, err := encoding.NewEncoder(ctx, &build.File{Filename: "o.vp2", Encoding: "vp2", Form: "data"}, &encoding.Config{Mode: filetypes.Export, Out: &buf2})
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNotNil(enc2.Encode(nonConcrete)))
}

// TestValuePlaneModeMatrix mirrors the built-in mode-matrix parity: a
// value-plane registration resolves by extension and qualifier in all
// four modes.
func TestValuePlaneModeMatrix(t *testing.T) {
	reset(t)
	err := encodingregistry.Register(encodingregistry.Encoding{
		Name:            "vpm",
		Extensions:      []string{".vpm"},
		Info:            encodingregistry.FileInfo{Form: "schema"},
		NewValueDecoder: newVPDecoder,
		NewValueEncoder: newVPEncoder,
	})
	qt.Assert(t, qt.IsNil(err))

	for _, mode := range []filetypes.Mode{filetypes.Input, filetypes.Export, filetypes.Def, filetypes.Eval} {
		byExt, err := filetypes.ParseFile("x.vpm", mode)
		qt.Assert(t, qt.IsNil(err))
		qt.Assert(t, qt.Equals(byExt.Encoding, build.Encoding("vpm")))

		byQual, err := filetypes.ParseFile("vpm:x.other", mode)
		qt.Assert(t, qt.IsNil(err))
		qt.Assert(t, qt.Equals(byQual.Encoding, build.Encoding("vpm")))
	}
}

// TestValuePlaneValidation checks mutual-exclusion and one-decoder rules.
func TestValuePlaneValidation(t *testing.T) {
	dec := newKVDecoder

	t.Run("both-decoders", func(t *testing.T) {
		reset(t)
		err := encodingregistry.Register(encodingregistry.Encoding{
			Name:            "x",
			NewDecoder:      dec,
			NewValueDecoder: newVPDecoder,
		})
		qt.Assert(t, qt.ErrorMatches(err, `.*NewDecoder and NewValueDecoder are mutually exclusive`))
	})

	t.Run("both-encoders", func(t *testing.T) {
		reset(t)
		err := encodingregistry.Register(encodingregistry.Encoding{
			Name:            "x",
			NewValueDecoder: newVPDecoder,
			NewEncoder:      newKVEncoder,
			NewValueEncoder: newVPEncoder,
		})
		qt.Assert(t, qt.ErrorMatches(err, `.*NewEncoder and NewValueEncoder are mutually exclusive`))
	})

	t.Run("no-decoder", func(t *testing.T) {
		reset(t)
		err := encodingregistry.Register(encodingregistry.Encoding{Name: "x"})
		qt.Assert(t, qt.ErrorMatches(err, `.*a decoder is required \(NewDecoder or NewValueDecoder\)`))
	})
}
