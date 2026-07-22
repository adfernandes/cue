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
	"fmt"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/internal/filetypes"
)

// This file validates the declarative part of a registration against
// the types.cue #FileInfo template — the same template the built-in
// file-type data is generated from — and converts it to the plain-data
// form the internal registry consumes. The CUE evaluator dependency of
// this whole feature lives here and nowhere else: the resolution path
// in internal/filetypes never evaluates CUE (research.md §8.5).

// template lazily compiles the embedded types.cue source and resolves
// its #FileInfo template. Compiled once per process; each registration
// then costs one unification against it.
type template struct {
	ctx  *cue.Context
	tmpl cue.Value
}

var templateValue = sync.OnceValues(compileTemplate)

// compileTemplate constructs the types.cue validation template. Keeping the
// cold operation separate makes its startup cost directly benchmarkable;
// production calls still go through templateValue and compile it at most once.
func compileTemplate() (template, error) {
	ctx := cuecontext.New()
	v := ctx.CompileBytes(filetypes.TypesCUESource(), cue.Filename("types.cue"))
	if err := v.Err(); err != nil {
		return template{}, err
	}
	tmpl := v.LookupPath(cue.MakePath(cue.Def("#FileInfo")))
	if err := tmpl.Err(); err != nil {
		return template{}, err
	}
	return template{ctx: ctx, tmpl: tmpl}, nil
}

var modeToInternal = map[Mode]filetypes.Mode{
	Input:  filetypes.Input,
	Export: filetypes.Export,
	Def:    filetypes.Def,
	Eval:   filetypes.Eval,
}

func register(e Encoding) error {
	if e.Name == "" {
		return fmt.Errorf("cannot register encoding: name must be non-empty")
	}
	// The name is also the file qualifier, parsed from strings such as
	// "name+tag=value:". A name containing a qualifier separator could
	// never be matched as a qualifier, so reject it rather than register
	// an unusable encoding.
	if i := strings.IndexAny(e.Name, "+=:"); i >= 0 {
		return fmt.Errorf("cannot register encoding %q: name must not contain %q", e.Name, e.Name[i])
	}
	if e.NewDecoder == nil {
		return fmt.Errorf("cannot register encoding %q: NewDecoder is required", e.Name)
	}
	for _, ext := range e.Extensions {
		if !strings.HasPrefix(ext, ".") || ext == "." {
			return fmt.Errorf("cannot register encoding %q: extension %q must start with a dot and name a suffix", e.Name, ext)
		}
	}
	// A subsidiary tag cannot share the encoding's own name (the name is
	// a top-level qualifier), and a tag cannot be both string- and
	// boolean-valued: either leaves a tag that resolution cannot use.
	for name := range e.Tags {
		if name == e.Name {
			return fmt.Errorf("cannot register encoding %q: tag %q cannot share the encoding name", e.Name, name)
		}
		if _, ok := e.BoolTags[name]; ok {
			return fmt.Errorf("cannot register encoding %q: tag %q declared as both string and boolean", e.Name, name)
		}
	}
	for name := range e.BoolTags {
		if name == e.Name {
			return fmt.Errorf("cannot register encoding %q: tag %q cannot share the encoding name", e.Name, name)
		}
	}

	// Validate the declarative record against the types.cue #FileInfo
	// template: the base properties and every per-mode overlay.
	if err := validateInfo(e.Name, e.Info); err != nil {
		return err
	}
	perMode := make(map[filetypes.Mode]filetypes.DynamicFileInfo, len(e.PerMode))
	for mode, info := range e.PerMode {
		m, ok := modeToInternal[mode]
		if !ok {
			return fmt.Errorf("cannot register encoding %q: unknown mode %q", e.Name, mode)
		}
		if err := validateInfo(e.Name, info); err != nil {
			return err
		}
		perMode[m] = internalInfo(info)
	}

	d := filetypes.DynamicEncoding{
		Name:       e.Name,
		Extensions: e.Extensions,
		Info:       internalInfo(e.Info),
		PerMode:    perMode,
		Codec: filetypes.Codec{
			NewDecoder: e.NewDecoder,
			NewEncoder: e.NewEncoder,
			Binary:     e.Binary,
		},
	}
	if len(e.Tags) > 0 {
		d.Tags = make(map[string]*string, len(e.Tags))
		for name, decl := range e.Tags {
			d.Tags[name] = decl.Default
		}
	}
	if len(e.BoolTags) > 0 {
		d.BoolTags = make(map[string]*bool, len(e.BoolTags))
		for name, decl := range e.BoolTags {
			d.BoolTags[name] = decl.Default
		}
	}
	return filetypes.RegisterEncoding(d)
}

// validateInfo unifies one FileInfo declaration with the types.cue
// #FileInfo template and reports any conflict as a registration error.
func validateInfo(name string, info FileInfo) error {
	t, err := templateValue()
	if err != nil {
		return fmt.Errorf("cannot register encoding %q: internal template error: %v", name, err)
	}
	decl := map[string]any{"encoding": name}
	if info.Interpretation != "" {
		decl["interpretation"] = info.Interpretation
	}
	if info.Form != "" {
		decl["form"] = info.Form
	}
	for aspect, v := range info.Aspects {
		aspectName := string(aspect)
		if _, ok := decl[aspectName]; ok {
			return fmt.Errorf("cannot register encoding %q: aspect %q conflicts with declared field", name, aspect)
		}
		decl[aspectName] = v
	}
	v := t.tmpl.Unify(t.ctx.Encode(decl))
	if err := v.Err(); err != nil {
		return fmt.Errorf("cannot register encoding %q: %v", name, err)
	}
	if err := v.Validate(); err != nil {
		return fmt.Errorf("cannot register encoding %q: %v", name, err)
	}
	return nil
}

func internalInfo(info FileInfo) filetypes.DynamicFileInfo {
	out := filetypes.DynamicFileInfo{
		Interpretation: info.Interpretation,
		Form:           info.Form,
	}
	if info.Aspects != nil {
		out.Aspects = make(map[string]bool, len(info.Aspects))
		for aspect, value := range info.Aspects {
			out.Aspects[string(aspect)] = value
		}
	}
	return out
}
