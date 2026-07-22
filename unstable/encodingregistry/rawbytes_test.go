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
	"io"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/internal/encoding"
	"cuelang.org/go/internal/filetypes"
	"cuelang.org/go/unstable/encodingregistry"
	"github.com/go-quicktest/qt"
)

// captureDecoder records the exact bytes its reader yields, so a test
// can assert whether internal/encoding handed the decoder raw or
// UTF/BOM-transformed input.
type captureDecoder struct {
	got  *[]byte
	done bool
}

func (d *captureDecoder) Decode() (ast.Expr, error) {
	if d.done {
		return nil, io.EOF
	}
	d.done = true
	return ast.NewString(string(*d.got)), nil
}

func newCaptureDecoder(got *[]byte) func(*build.File, io.Reader) (encodingregistry.Decoder, error) {
	return func(f *build.File, r io.Reader) (encodingregistry.Decoder, error) {
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		*got = b
		return &captureDecoder{got: got}, nil
	}
}

// TestBinaryEncodingRawBytes checks that a registration declaring
// Binary: true receives its input bytes untouched — no BOM stripping,
// no UTF-16 transcoding, no U+FFFD replacement of invalid UTF-8 — while
// a non-binary registration still goes through the UTF-8/BOM transform.
func TestBinaryEncodingRawBytes(t *testing.T) {
	// Bytes that the UTF-8/BOM transform would mangle: a UTF-16LE BOM
	// followed by content, plus a lone invalid byte.
	raw := []byte{0xff, 0xfe, 'a', '=', 'b', 0x80, '\n'}

	t.Run("binary-is-raw", func(t *testing.T) {
		filetypes.ResetDynamicRegistryForTesting()
		t.Cleanup(filetypes.ResetDynamicRegistryForTesting)

		var got []byte
		err := encodingregistry.Register(encodingregistry.Encoding{
			Name:       "rawbin",
			Extensions: []string{".rawbin"},
			Binary:     true,
			Info:       encodingregistry.FileInfo{Form: "data"},
			NewDecoder: newCaptureDecoder(&got),
		})
		qt.Assert(t, qt.IsNil(err))

		f, err := filetypes.ParseFile("x.rawbin", filetypes.Input)
		qt.Assert(t, qt.IsNil(err))
		f.Source = raw

		ctx := cuecontext.New()
		dec := encoding.NewDecoder(ctx, f, nil)
		defer dec.Close()
		qt.Assert(t, qt.IsNil(dec.Err()))

		qt.Assert(t, qt.DeepEquals(got, raw))
	})

	t.Run("non-binary-is-transformed", func(t *testing.T) {
		filetypes.ResetDynamicRegistryForTesting()
		t.Cleanup(filetypes.ResetDynamicRegistryForTesting)

		var got []byte
		err := encodingregistry.Register(encodingregistry.Encoding{
			Name:       "textkv",
			Extensions: []string{".textkv"},
			Info:       encodingregistry.FileInfo{Form: "data"},
			NewDecoder: newCaptureDecoder(&got),
		})
		qt.Assert(t, qt.IsNil(err))

		f, err := filetypes.ParseFile("x.textkv", filetypes.Input)
		qt.Assert(t, qt.IsNil(err))
		f.Source = raw

		ctx := cuecontext.New()
		dec := encoding.NewDecoder(ctx, f, nil)
		defer dec.Close()
		qt.Assert(t, qt.IsNil(dec.Err()))

		// The transform alters the bytes, so a non-binary encoding must
		// NOT receive the original input verbatim.
		qt.Assert(t, qt.Not(qt.DeepEquals(got, raw)))
	})
}
