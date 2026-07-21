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

// Package prototype is DISPOSABLE RESEARCH CODE for the
// specs/ace8n-dynamic-filetypes change (task T010). It prototypes the
// C2+C3+C4 architecture direction of decision D-005:
//
//   - C2: open per-encoding data structures (maps keyed by name) instead
//     of the whole-tag-space binary tables and baked enum slices;
//   - C3: runtime unification logic (mode ⊓ tags ⊓ extension ⊓
//     subsidiary tags) executing over those structures — hand-written
//     here as a stand-in for what a generator would emit from types.cue;
//   - C4: population and registration-time validation performed once by
//     the real CUE evaluator against types.cue, never per resolution.
//
// It is imported by nothing outside its own tests and is removed (or
// superseded by the real implementation) per task T039. Divergences
// from the shipping implementation are acceptable and are recorded by
// the parity test (T011).
package prototype

import (
	"fmt"
	"maps"
	"path/filepath"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/internal/filetypes/internal"
)

// kind describes how much we know about a scalar property value.
type kind uint8

const (
	unset    kind = iota // no value (absent or pure constraint like !="")
	dflt                 // disjunction with a default; can be overridden
	concrete             // fixed value; conflicting values are an error
)

// val is a tri-state scalar property. The bool returned by unify reports
// compatibility: true means the two values unified without conflict.
type val[T comparable] struct {
	k kind
	v T
}

// sval is a string-valued property (encoding, interpretation, form, or a
// string tag value).
type sval = val[string]

// bval is a boolean-valued property (an aspect or boolean tag).
type bval = val[bool]

func (a val[T]) unify(b val[T]) (val[T], bool) {
	switch {
	case a.k == unset:
		return b, true
	case b.k == unset:
		return a, true
	case a.k == concrete && b.k == concrete:
		return a, a.v == b.v
	case a.k == concrete:
		return a, true // concrete beats default
	case b.k == concrete:
		return b, true
	default: // both defaults
		return a, a.v == b.v
	}
}

func (a val[T]) resolve() T {
	if a.k == unset {
		var zero T
		return zero
	}
	return a.v
}

// entry is the open representation of one types.cue #FileInfo value:
// a mode base, a tag, an extension, an encoding, a form, or an
// interpretation. This is the data structure a registration inserts.
type entry struct {
	encoding       sval
	interpretation sval
	form           sval

	aspects [numAspects]bval

	// boolTags and tags double as the allow-list for subsidiary tags:
	// a subsidiary tag is permitted iff its key is present.
	boolTags map[string]bval
	tags     map[string]sval

	// alts holds the disjuncts of a struct-level disjunction (only
	// tagInfo.pb in today's types.cue). The entry's own fields hold the
	// default disjunct; alts the remaining ones, tried on conflict.
	alts []*entry
}

const (
	aAttributes = iota
	aConstraints
	aCycles
	aData
	aDefinitions
	aDocs
	aIncomplete
	aImports
	aKeepDefaults
	aOptional
	aReferences
	aStream
	numAspects
)

var aspectNames = [numAspects]string{
	aAttributes:   "attributes",
	aConstraints:  "constraints",
	aCycles:       "cycles",
	aData:         "data",
	aDefinitions:  "definitions",
	aDocs:         "docs",
	aIncomplete:   "incomplete",
	aImports:      "imports",
	aKeepDefaults: "keepDefaults",
	aOptional:     "optional",
	aReferences:   "references",
	aStream:       "stream",
}

func (e *entry) clone() *entry {
	c := *e
	c.boolTags = maps.Clone(e.boolTags)
	c.tags = maps.Clone(e.tags)
	return &c
}

// unify merges b into e (e is mutated). It reports an error naming
// field on the first conflict.
func (e *entry) unify(b *entry) error {
	if err := e.unify1(b); err != nil {
		return err
	}
	return nil
}

func (e *entry) unify1(b *entry) error {
	if len(b.alts) > 0 {
		// Try the default disjunct first, then the alternatives.
		save := e.clone()
		noAlts := b.clone()
		noAlts.alts = nil
		if err := e.unify1(noAlts); err == nil {
			return nil
		}
		*e = *save
		for _, alt := range b.alts {
			save := e.clone()
			if err := e.unify1(alt); err == nil {
				return nil
			}
			*e = *save
		}
		return fmt.Errorf("no disjunct of value matches")
	}
	var ok bool
	if e.encoding, ok = e.encoding.unify(b.encoding); !ok {
		return fmt.Errorf("conflicting values for encoding")
	}
	if e.interpretation, ok = e.interpretation.unify(b.interpretation); !ok {
		return fmt.Errorf("conflicting values for interpretation")
	}
	if e.form, ok = e.form.unify(b.form); !ok {
		return fmt.Errorf("conflicting values for form")
	}
	for i := range e.aspects {
		if e.aspects[i], ok = e.aspects[i].unify(b.aspects[i]); !ok {
			return fmt.Errorf("conflicting values for %s", aspectNames[i])
		}
	}
	for k, v := range b.boolTags {
		if e.boolTags == nil {
			e.boolTags = map[string]bval{}
		}
		if old, exists := e.boolTags[k]; exists {
			nv, ok := old.unify(v)
			if !ok {
				return fmt.Errorf("conflicting values for boolTags.%s", k)
			}
			e.boolTags[k] = nv
		} else {
			e.boolTags[k] = v
		}
	}
	for k, v := range b.tags {
		if e.tags == nil {
			e.tags = map[string]sval{}
		}
		if old, exists := e.tags[k]; exists {
			nv, ok := old.unify(v)
			if !ok {
				return fmt.Errorf("conflicting values for tags.%s", k)
			}
			e.tags[k] = nv
		} else {
			e.tags[k] = v
		}
	}
	return nil
}

// Mode mirrors filetypes.Mode.
type Mode int

const (
	Input Mode = iota
	Export
	Def
	Eval
	NumModes
)

var modeNames = [NumModes]string{"input", "export", "def", "eval"}

// Registry holds the open per-encoding data (C2). Built-in entries are
// populated from types.cue by Load; dynamic entries are inserted by
// Register after validation against the same template (C4).
type Registry struct {
	base       [NumModes]*entry            // modes[m].FileInfo
	extensions [NumModes]map[string]*entry // modes[m].extensions
	encodings  [NumModes]map[string]*entry // modes[m].encodings
	tagInfo    map[string]*entry
	forms      map[string]*entry
	interps    map[string]*entry

	// tag classification for scope parsing, replacing the generated
	// tagTypes map. Derived openly from the data.
	subsidiaryBool   map[string]bool
	subsidiaryString map[string]bool

	// template retained for registration-time validation (C4).
	template cue.Value
}

// Load populates a Registry from types.cue source using the CUE
// evaluator. This is a one-time cost, the analog of registration-time
// evaluation; resolution never evaluates CUE.
func Load(typesSource []byte) (*Registry, error) {
	ctx := cuecontext.New()
	v := ctx.CompileBytes(typesSource, cue.Filename("types.cue"))
	if err := v.Err(); err != nil {
		return nil, err
	}
	r := &Registry{
		tagInfo:          map[string]*entry{},
		forms:            map[string]*entry{},
		interps:          map[string]*entry{},
		subsidiaryBool:   map[string]bool{},
		subsidiaryString: map[string]bool{},
		template:         v.LookupPath(cue.MakePath(cue.Def("#FileInfo"))),
	}
	for m := Input; m < NumModes; m++ {
		mv := v.LookupPath(cue.MakePath(cue.Str("modes"), cue.Str(modeNames[m])))
		if err := mv.Err(); err != nil {
			return nil, err
		}
		e, err := extractEntry(mv.LookupPath(cue.MakePath(cue.Str("FileInfo"))))
		if err != nil {
			return nil, err
		}
		r.base[m] = e
		r.extensions[m] = map[string]*entry{}
		if err := extractMap(mv, "extensions", r.extensions[m]); err != nil {
			return nil, err
		}
		r.encodings[m] = map[string]*entry{}
		if err := extractMap(mv, "encodings", r.encodings[m]); err != nil {
			return nil, err
		}
	}
	if err := extractMap(v, "tagInfo", r.tagInfo); err != nil {
		return nil, err
	}
	if err := extractMap(v, "forms", r.forms); err != nil {
		return nil, err
	}
	if err := extractMap(v, "interpretations", r.interps); err != nil {
		return nil, err
	}
	// Classify subsidiary tags from the open data.
	all := []map[string]*entry{r.tagInfo, r.forms, r.interps}
	for m := Input; m < NumModes; m++ {
		all = append(all, r.encodings[m], r.extensions[m])
	}
	for _, mp := range all {
		for _, e := range mp {
			for k := range e.boolTags {
				r.subsidiaryBool[k] = true
			}
			for k := range e.tags {
				r.subsidiaryString[k] = true
			}
		}
	}
	return r, nil
}

func extractMap(v cue.Value, field string, dst map[string]*entry) error {
	fv := v.LookupPath(cue.MakePath(cue.Str(field)))
	if !fv.Exists() {
		return nil
	}
	iter, err := fv.Fields()
	if err != nil {
		return err
	}
	for iter.Next() {
		e, err := extractEntry(iter.Value())
		if err != nil {
			return fmt.Errorf("%s.%s: %v", field, iter.Selector(), err)
		}
		dst[iter.Selector().Unquoted()] = e
	}
	return nil
}

// extractEntry converts one evaluated #FileInfo-shaped CUE value into
// the open Go representation, preserving default-ness per field.
func extractEntry(v cue.Value) (*entry, error) {
	if op, args := v.Expr(); op == cue.OrOp && len(args) > 1 {
		// Struct-level disjunction (tagInfo.pb). The default disjunct
		// becomes the entry itself; the rest become alternatives.
		d, hasDefault := v.Default()
		if !hasDefault {
			return nil, fmt.Errorf("struct disjunction without default")
		}
		e, err := extractEntry(d)
		if err != nil {
			return nil, err
		}
		for _, a := range args {
			// Skip the disjunct that equals the default.
			if a.Subsume(d) == nil && d.Subsume(a) == nil {
				continue
			}
			alt, err := extractEntry(a)
			if err != nil {
				return nil, err
			}
			e.alts = append(e.alts, alt)
		}
		return e, nil
	}
	e := &entry{}
	var err error
	if e.encoding, err = extractStr(v, "encoding"); err != nil {
		return nil, err
	}
	if e.interpretation, err = extractStr(v, "interpretation"); err != nil {
		return nil, err
	}
	if e.form, err = extractStr(v, "form"); err != nil {
		return nil, err
	}
	for i := range e.aspects {
		if e.aspects[i], err = extractBool(v, aspectNames[i]); err != nil {
			return nil, err
		}
	}
	if e.boolTags, err = extractBoolMap(v, "boolTags"); err != nil {
		return nil, err
	}
	if e.tags, err = extractStrMap(v, "tags"); err != nil {
		return nil, err
	}
	return e, nil
}

func extractStr(v cue.Value, field string) (sval, error) {
	f := v.LookupPath(cue.MakePath(cue.Str(field)))
	if !f.Exists() {
		return sval{}, nil
	}
	if d, ok := f.Default(); ok {
		s, err := d.String()
		if err != nil {
			return sval{}, nil // default is not a concrete string; treat open
		}
		if f.IsConcrete() && f.Kind() == cue.StringKind {
			return sval{concrete, s}, nil
		}
		return sval{dflt, s}, nil
	}
	if f.IsConcrete() && f.Kind() == cue.StringKind {
		s, err := f.String()
		if err != nil {
			return sval{}, err
		}
		return sval{concrete, s}, nil
	}
	return sval{}, nil // pure constraint (!="", string, _): open
}

func extractBool(v cue.Value, field string) (bval, error) {
	f := v.LookupPath(cue.MakePath(cue.Str(field)))
	if !f.Exists() {
		return bval{}, nil
	}
	if d, ok := f.Default(); ok {
		b, err := d.Bool()
		if err != nil {
			return bval{}, nil
		}
		if f.IsConcrete() && f.Kind() == cue.BoolKind {
			return bval{concrete, b}, nil
		}
		return bval{dflt, b}, nil
	}
	if f.IsConcrete() && f.Kind() == cue.BoolKind {
		b, err := f.Bool()
		if err != nil {
			return bval{}, err
		}
		return bval{concrete, b}, nil
	}
	return bval{}, nil
}

func extractBoolMap(v cue.Value, field string) (map[string]bval, error) {
	f := v.LookupPath(cue.MakePath(cue.Str(field)))
	if !f.Exists() {
		return nil, nil
	}
	iter, err := f.Fields()
	if err != nil {
		return nil, err
	}
	m := map[string]bval{}
	for iter.Next() {
		b, err := extractBool(f, iter.Selector().Unquoted())
		if err != nil {
			return nil, err
		}
		m[iter.Selector().Unquoted()] = b
	}
	return m, nil
}

func extractStrMap(v cue.Value, field string) (map[string]sval, error) {
	f := v.LookupPath(cue.MakePath(cue.Str(field)))
	if !f.Exists() {
		return nil, nil
	}
	iter, err := f.Fields()
	if err != nil {
		return nil, err
	}
	m := map[string]sval{}
	for iter.Next() {
		s, err := extractStr(f, iter.Selector().Unquoted())
		if err != nil {
			return nil, err
		}
		m[iter.Selector().Unquoted()] = s
	}
	return m, nil
}

// Scope mirrors the filetypes scope: parsed tag sets from a qualifier.
type Scope struct {
	topLevel         []string
	subsidiaryBool   map[string]bool
	subsidiaryString map[string]string
}

// ParseScope parses a qualifier like "json+schema" using the open tag
// classification (replacing the generated tagTypes gate).
func (r *Registry) ParseScope(s string) (*Scope, error) {
	sc := &Scope{}
	if s == "" {
		return sc, nil
	}
	for tag := range strings.SplitSeq(s, "+") {
		name, val, hasValue := strings.Cut(tag, "=")
		switch {
		case r.tagInfo[name] != nil:
			if hasValue {
				return nil, fmt.Errorf("cannot specify value for tag %q", name)
			}
			sc.topLevel = append(sc.topLevel, name)
		case r.subsidiaryBool[name]:
			b := true
			if hasValue {
				var err error
				if b, err = strconv.ParseBool(val); err != nil {
					return nil, fmt.Errorf("invalid boolean value for tag %q", name)
				}
			}
			if sc.subsidiaryBool == nil {
				sc.subsidiaryBool = map[string]bool{}
			}
			sc.subsidiaryBool[name] = b
		case r.subsidiaryString[name]:
			if !hasValue {
				return nil, fmt.Errorf("tag %q must have value (%s=<value>)", name, name)
			}
			if sc.subsidiaryString == nil {
				sc.subsidiaryString = map[string]string{}
			}
			sc.subsidiaryString[name] = val
		default:
			return nil, fmt.Errorf("unknown filetype %s", name)
		}
	}
	return sc, nil
}

func fileExt(f string) string {
	if f == "-" {
		return "-"
	}
	e := filepath.Ext(f)
	if e == "" || e == filepath.Base(f) {
		return ""
	}
	return e
}

// ToFile is the prototype equivalent of filetypes.ParseFileAndType
// after scope parsing: mode ⊓ tags ⊓ extension ⊓ subsidiary tags.
func (r *Registry) ToFile(mode Mode, sc *Scope, filename string) (*build.File, error) {
	e := r.base[mode].clone()
	for _, tag := range sc.topLevel {
		ti := r.tagInfo[tag]
		if ti == nil {
			return nil, fmt.Errorf("unknown filetype %s", tag)
		}
		if err := e.unify(ti); err != nil {
			return nil, err
		}
	}
	if e.encoding.k == unset {
		if ext := fileExt(filename); ext != "" {
			ee := r.extensions[mode][ext]
			if ee == nil {
				return nil, fmt.Errorf("unknown file extension %s", ext)
			}
			if err := e.unify(ee); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("no encoding specified for file %q", filename)
		}
	}
	for tag, val := range sc.subsidiaryBool {
		if _, ok := e.boolTags[tag]; !ok {
			return nil, fmt.Errorf("tag %s is not allowed in this context", tag)
		}
		e.boolTags[tag] = bval{concrete, val}
	}
	for tag, val := range sc.subsidiaryString {
		if _, ok := e.tags[tag]; !ok {
			return nil, fmt.Errorf("tag %s is not allowed in this context", tag)
		}
		e.tags[tag] = sval{concrete, val}
	}
	f := &build.File{
		Filename:       filename,
		Encoding:       build.Encoding(e.encoding.resolve()),
		Interpretation: build.Interpretation(e.interpretation.resolve()),
		Form:           build.Form(e.form.resolve()),
	}
	for k, v := range e.tags {
		if v.k != unset {
			if f.Tags == nil {
				f.Tags = map[string]string{}
			}
			f.Tags[k] = v.resolve()
		}
	}
	for k, v := range e.boolTags {
		if v.k != unset {
			if f.BoolTags == nil {
				f.BoolTags = map[string]bool{}
			}
			f.BoolTags[k] = v.resolve()
		}
	}
	return f, nil
}

// FromFile is the prototype equivalent of filetypes.FromFile.
func (r *Registry) FromFile(b *build.File, mode Mode) (*internal.FileInfo, error) {
	if b.Encoding == "" {
		return nil, fmt.Errorf("no encoding specified")
	}
	e := r.base[mode].clone()
	var ok bool
	if e.encoding, ok = e.encoding.unify(sval{concrete, string(b.Encoding)}); !ok {
		return nil, fmt.Errorf("conflicting values for encoding")
	}
	if b.Interpretation != "" {
		if e.interpretation, ok = e.interpretation.unify(sval{concrete, string(b.Interpretation)}); !ok {
			return nil, fmt.Errorf("conflicting values for interpretation")
		}
	}
	if b.Form != "" {
		if e.form, ok = e.form.unify(sval{concrete, string(b.Form)}); !ok {
			return nil, fmt.Errorf("conflicting values for form")
		}
	}
	interp := e.interpretation.resolve()
	if b.Form != "" {
		fe := r.forms[string(b.Form)]
		if fe == nil {
			return nil, fmt.Errorf("unknown forms %s", b.Form)
		}
		if err := e.unify(fe); err != nil {
			return nil, fmt.Errorf("unknown forms %s", b.Form)
		}
	} else if interp != "" {
		ie := r.interps[interp]
		if ie == nil {
			return nil, fmt.Errorf("unknown interpretations %s", interp)
		}
		if err := e.unify(ie); err != nil {
			return nil, fmt.Errorf("unknown interpretations %s", interp)
		}
	}
	if interp == "" {
		ee := r.encodings[mode][string(b.Encoding)]
		if ee == nil {
			return nil, fmt.Errorf("unknown encodings %s", b.Encoding)
		}
		if err := e.unify(ee); err != nil {
			return nil, fmt.Errorf("unknown encodings %s", b.Encoding)
		}
	}
	fi := &internal.FileInfo{
		Filename:       b.Filename,
		Encoding:       build.Encoding(e.encoding.resolve()),
		Interpretation: build.Interpretation(e.interpretation.resolve()),
		Form:           build.Form(e.form.resolve()),
		Data:           e.aspects[aData].resolve(),
		References:     e.aspects[aReferences].resolve(),
		Cycles:         e.aspects[aCycles].resolve(),
		Definitions:    e.aspects[aDefinitions].resolve(),
		Optional:       e.aspects[aOptional].resolve(),
		Constraints:    e.aspects[aConstraints].resolve(),
		KeepDefaults:   e.aspects[aKeepDefaults].resolve(),
		Incomplete:     e.aspects[aIncomplete].resolve(),
		Imports:        e.aspects[aImports].resolve(),
		Stream:         e.aspects[aStream].resolve(),
		Docs:           e.aspects[aDocs].resolve(),
		Attributes:     e.aspects[aAttributes].resolve(),
	}
	return fi, nil
}

// ParseFile mirrors filetypes.ParseFile for the benchmark operations.
func (r *Registry) ParseFile(s string, mode Mode) (*build.File, error) {
	scope, file, found := "", s, false
	if before, after, ok := strings.Cut(s, ":"); ok && !filepath.IsAbs(s) {
		scope, file, found = before, after, true
	}
	if found && scope == "" {
		return nil, fmt.Errorf("empty filetype prefix in %q", s)
	}
	if file == "" {
		return nil, fmt.Errorf("empty file name")
	}
	return r.ParseFileAndType(file, scope, mode)
}

// ParseFileAndType mirrors filetypes.ParseFileAndType.
func (r *Registry) ParseFileAndType(file, scope string, mode Mode) (*build.File, error) {
	sc, err := r.ParseScope(scope)
	if err != nil {
		return nil, err
	}
	return r.ToFile(mode, sc, file)
}

// ParseArgs mirrors filetypes.ParseArgs for the benchmark operation.
func (r *Registry) ParseArgs(args []string) (files []*build.File, err error) {
	sc := &Scope{}
	for _, s := range args {
		scope, file, found := "", s, false
		if before, after, ok := strings.Cut(s, ":"); ok {
			scope, file, found = before, after, true
		}
		switch {
		case !found:
			f, err := r.ToFile(Input, sc, file)
			if err != nil {
				return nil, err
			}
			files = append(files, f)
		case file == "":
			if sc, err = r.ParseScope(scope); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("cannot combine file type and file name")
		}
	}
	return files, nil
}

// Register validates a dynamic encoding declaration against the
// types.cue #FileInfo template with the real evaluator (C4) and, on
// success, inserts the resulting entry into the open structures (C2).
// Conflicts are refused add-only (D-002).
func (r *Registry) Register(name string, extensions []string, declSource []byte) error {
	if _, ok := r.tagInfo[name]; ok {
		return fmt.Errorf("cannot register encoding %q: conflicts with existing encoding %q", name, name)
	}
	for _, ext := range extensions {
		for m := Input; m < NumModes; m++ {
			if _, ok := r.extensions[m][ext]; ok {
				return fmt.Errorf("cannot register encoding %q: extension %q already registered", name, ext)
			}
		}
	}
	ctx := r.template.Context()
	decl := ctx.CompileBytes(declSource, cue.Filename(name+".cue"))
	if err := decl.Err(); err != nil {
		return fmt.Errorf("invalid registration for %q: %v", name, err)
	}
	unified := r.template.Unify(decl)
	if err := unified.Validate(); err != nil {
		return fmt.Errorf("registration for %q does not conform to the file-type template: %v", name, err)
	}
	e, err := extractEntry(unified)
	if err != nil {
		return err
	}
	r.tagInfo[name] = e
	for _, ext := range extensions {
		for m := Input; m < NumModes; m++ {
			r.extensions[m][ext] = e
			r.encodings[m][name] = e
		}
	}
	for k := range e.boolTags {
		r.subsidiaryBool[k] = true
	}
	for k := range e.tags {
		r.subsidiaryString[k] = true
	}
	return nil
}
