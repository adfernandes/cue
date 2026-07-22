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
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
)

// TestFreezeAtomicity stresses the registry freeze against a concurrent
// registration. The freeze contract is that once the registry is frozen
// by its first use, all resolutions observe the same registrations. The
// observable invariant: if RegisterEncoding returns nil (success), then
// a resolution racing with it must see the encoding — a nil return must
// never coexist with a resolution that froze a snapshot lacking it.
func TestFreezeAtomicity(t *testing.T) {
	const trials = 4000
	for i := 0; i < trials; i++ {
		ResetDynamicRegistryForTesting()

		var wg sync.WaitGroup
		start := make(chan struct{})
		var regErr error
		var resolveErr error

		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			regErr = RegisterEncoding(DynamicEncoding{
				Name:       "zz",
				Extensions: []string{".zz"},
				Info:       DynamicFileInfo{Form: "data"},
				Codec:      Codec{NewDecoder: stubDecoder},
			})
		}()
		go func() {
			defer wg.Done()
			<-start
			_, resolveErr = ParseFile("probe.zz", Input)
		}()
		close(start)
		wg.Wait()

		// If registration succeeded, the racing resolution must have
		// seen the encoding (no "unknown file extension" error).
		if regErr == nil && resolveErr != nil {
			t.Fatalf("trial %d: RegisterEncoding succeeded but a racing resolution did not see it: %v", i, resolveErr)
		}
	}
	ResetDynamicRegistryForTesting()
}

func stubDecoder(f *build.File, r io.Reader) (Decoder, error) {
	return stubDec{}, nil
}

type stubDec struct{}

func (stubDec) Decode() (ast.Expr, error) { return ast.NewNull(), io.EOF }

func TestBoundedCacheFIFO(t *testing.T) {
	t.Run("eviction-and-clear", func(t *testing.T) {
		cache := newBoundedCache[int, string](2)
		if got, loaded := cache.LoadOrStore(1, "one"); loaded || got != "one" {
			t.Fatalf("first store = (%q, %v); want (one, false)", got, loaded)
		}
		cache.LoadOrStore(2, "two")
		if got, loaded := cache.LoadOrStore(1, "replacement"); !loaded || got != "one" {
			t.Fatalf("duplicate store = (%q, %v); want (one, true)", got, loaded)
		}
		cache.LoadOrStore(3, "three")

		if _, ok := cache.Load(1); ok {
			t.Fatal("oldest entry was not evicted")
		}
		for key, want := range map[int]string{2: "two", 3: "three"} {
			if got, ok := cache.Load(key); !ok || got != want {
				t.Errorf("Load(%d) = (%q, %v); want (%q, true)", key, got, ok, want)
			}
		}
		if got := cache.Len(); got != 2 {
			t.Fatalf("Len() = %d; want 2", got)
		}

		cache.Clear()
		if got := cache.Len(); got != 0 {
			t.Fatalf("Len() after Clear = %d; want 0", got)
		}
		if _, ok := cache.Load(2); ok {
			t.Fatal("Clear retained an entry")
		}
	})

	t.Run("concurrent-admission-keeps-map-and-fifo-aligned", func(t *testing.T) {
		const (
			limit = 8
			count = 64
		)
		cache := newBoundedCache[int, int](limit)
		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cache.LoadOrStore(i, i)
			}()
		}
		wg.Wait()

		if got := cache.Len(); got != limit {
			t.Fatalf("Len() = %d; want %d", got, limit)
		}
		ranged := 0
		cache.Range(func(key, value int) bool {
			ranged++
			if key != value {
				t.Errorf("entry %d has value %d", key, value)
			}
			return true
		})
		if ranged != limit {
			t.Fatalf("Range visited %d entries; want %d", ranged, limit)
		}
		for _, key := range cache.keys {
			if _, ok := cache.Load(key); !ok {
				t.Fatalf("FIFO key %d is absent from map", key)
			}
		}
	})
}

// TestCacheBounded checks that resolutions with arbitrary subsidiary
// string tag values (e.g. code+lang=<anything>) do not accumulate in
// the resolution cache without bound: the value space is unbounded and
// attacker- or user-controllable in a long-running process.
func TestCacheBounded(t *testing.T) {
	t.Cleanup(ResetDynamicRegistryForTesting)

	cacheLen := toFileCache.Len

	t.Run("unbounded-string-values-are-not-cached", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		const n = 1000
		for i := 0; i < n; i++ {
			if _, err := ParseFileAndType("x", "code+lang=v"+strconv.Itoa(i), Input); err != nil {
				t.Fatalf("resolve %d: %v", i, err)
			}
		}
		if count := cacheLen(); count != 0 {
			t.Errorf("toFileCache grew to %d entries for %d distinct lang values; want 0", count, n)
		}
	})

	t.Run("top-level-tags-use-semantic-key", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		for _, qualifier := range []string{
			"json+schema",
			"schema+json",
			"json+schema+json",
			"schema+json+schema",
		} {
			if _, err := ParseFileAndType("x.data", qualifier, Input); err != nil {
				t.Fatalf("resolve %q: %v", qualifier, err)
			}
		}
		if count := cacheLen(); count != 1 {
			t.Errorf("equivalent top-level scopes created %d cache entries; want 1", count)
		}
	})

	t.Run("boolean-tags-use-semantic-key", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		for _, qualifier := range []string{
			"json+compact=false",
			"compact=false+json",
			"json+compact=0+json",
			"compact=False+json+compact=false",
		} {
			if _, err := ParseFileAndType("x.data", qualifier, Input); err != nil {
				t.Fatalf("resolve %q: %v", qualifier, err)
			}
		}
		if count := cacheLen(); count != 1 {
			t.Errorf("equivalent boolean-tag scopes created %d cache entries; want 1", count)
		}
	})

	t.Run("boolean-combinations-respect-hard-cap", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		const tagCount = 13 // 8192 combinations, greater than the cache cap.
		boolTags := make(map[string]*bool, tagCount)
		for i := 0; i < tagCount; i++ {
			boolTags["b"+strconv.Itoa(i)] = nil
		}
		if err := RegisterEncoding(DynamicEncoding{
			Name:       "manybools",
			Extensions: []string{".manybools"},
			BoolTags:   boolTags,
			Codec:      Codec{NewDecoder: stubDecoder},
		}); err != nil {
			t.Fatal(err)
		}
		var firstKey, newestKey toFileKey
		for combination := 0; combination < maxToFileCacheEntries+257; combination++ {
			var scope strings.Builder
			scope.WriteString("manybools")
			for bit := 0; bit < tagCount; bit++ {
				scope.WriteString("+b")
				scope.WriteString(strconv.Itoa(bit))
				scope.WriteByte('=')
				scope.WriteString(strconv.FormatBool(combination&(1<<bit) != 0))
			}
			sc, err := parseScope(scope.String())
			if err != nil {
				t.Fatalf("parse combination %d: %v", combination, err)
			}
			key := toFileKey{mode: Input, scope: sc.canonicalKey}
			if combination == 0 {
				firstKey = key
			}
			newestKey = key
			if _, err := toFile(Input, sc, "x"); err != nil {
				t.Fatalf("resolve combination %d: %v", combination, err)
			}
		}
		if count := cacheLen(); count != maxToFileCacheEntries {
			t.Errorf("toFileCache has %d entries after exceeding cap; want %d", count, maxToFileCacheEntries)
		}
		if _, ok := toFileCache.Load(firstKey); ok {
			t.Error("oldest resolution remained after FIFO overflow")
		}
		if _, ok := toFileCache.Load(newestKey); !ok {
			t.Error("newest resolution was not admitted after FIFO overflow")
		}
	})
}

func TestParsedScopeCache(t *testing.T) {
	t.Cleanup(ResetDynamicRegistryForTesting)

	cacheLen := parsedScopeCache.Len

	t.Run("reuses-raw-qualifier", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		first, err := parseScope("json+schema")
		if err != nil {
			t.Fatal(err)
		}
		second, err := parseScope("json+schema")
		if err != nil {
			t.Fatal(err)
		}
		if first != second {
			t.Fatal("repeated raw qualifier did not reuse its immutable parsed scope")
		}
		if got := cacheLen(); got != 1 {
			t.Fatalf("parsed-scope cache has %d entries; want 1", got)
		}
	})

	t.Run("canonical-resolution-key", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		first, err := parseScope("json+schema")
		if err != nil {
			t.Fatal(err)
		}
		second, err := parseScope("schema+json+schema")
		if err != nil {
			t.Fatal(err)
		}
		if first == second {
			t.Fatal("distinct raw qualifiers unexpectedly share a parsed-scope entry")
		}
		if first.canonicalKey != second.canonicalKey {
			t.Fatalf("equivalent qualifiers have canonical keys %q and %q", first.canonicalKey, second.canonicalKey)
		}
		for _, qualifier := range []string{"json+schema", "schema+json+schema"} {
			if _, err := ParseFileAndType("x.data", qualifier, Input); err != nil {
				t.Fatalf("ParseFileAndType(%q): %v", qualifier, err)
			}
		}
		resolved := toFileCache.Len()
		if resolved != 1 {
			t.Fatalf("equivalent parsed scopes created %d resolution entries; want 1", resolved)
		}
	})

	t.Run("unbounded-values-are-not-retained", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		first, err := parseScope("code+lang=arbitrary")
		if err != nil {
			t.Fatal(err)
		}
		second, err := parseScope("code+lang=arbitrary")
		if err != nil {
			t.Fatal(err)
		}
		if first == second {
			t.Fatal("string-valued qualifier unexpectedly reused a parsed-scope entry")
		}
		if got := cacheLen(); got != 0 {
			t.Fatalf("parsed-scope cache retained %d string-valued qualifiers; want 0", got)
		}
	})

	t.Run("long-raw-key-is-not-retained", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		qualifier := "json" + strings.Repeat("+schema", maxParsedScopeKeyBytes/len("+schema")+1)
		if len(qualifier) <= maxParsedScopeKeyBytes {
			t.Fatalf("test qualifier length %d does not exceed limit %d", len(qualifier), maxParsedScopeKeyBytes)
		}
		first, err := parseScope(qualifier)
		if err != nil {
			t.Fatal(err)
		}
		second, err := parseScope(qualifier)
		if err != nil {
			t.Fatal(err)
		}
		if first == second {
			t.Fatal("over-length qualifier unexpectedly reused a parsed-scope entry")
		}
		if got := cacheLen(); got != 0 {
			t.Fatalf("parsed-scope cache retained %d over-length qualifiers; want 0", got)
		}
	})

	t.Run("entry-cap", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		const tagSlots = 11 // 2^11 spellings, more than the cache cap.
		var firstRaw, newestRaw string
		var firstScope, newestScope *scope
		for i := 0; i < maxParsedScopeCacheEntries+17; i++ {
			var qualifier strings.Builder
			qualifier.WriteString("json")
			for bit := 0; bit < tagSlots; bit++ {
				if i&(1<<bit) == 0 {
					qualifier.WriteString("+json")
				} else {
					qualifier.WriteString("+schema")
				}
			}
			raw := qualifier.String()
			if len(raw) > maxParsedScopeKeyBytes {
				t.Fatalf("test qualifier length %d exceeds limit %d", len(raw), maxParsedScopeKeyBytes)
			}
			sc, err := parseScope(raw)
			if err != nil {
				t.Fatalf("parse spelling %d: %v", i, err)
			}
			if i == 0 {
				firstRaw, firstScope = raw, sc
			}
			newestRaw, newestScope = raw, sc
		}
		if got := cacheLen(); got != maxParsedScopeCacheEntries {
			t.Fatalf("parsed-scope cache has %d entries; want cap %d", got, maxParsedScopeCacheEntries)
		}
		again, err := parseScope(newestRaw)
		if err != nil {
			t.Fatal(err)
		}
		if again != newestScope {
			t.Fatal("newest qualifier was not retained after FIFO overflow")
		}
		again, err = parseScope(firstRaw)
		if err != nil {
			t.Fatal(err)
		}
		if again == firstScope {
			t.Fatal("oldest qualifier remained after FIFO overflow")
		}
	})

	t.Run("reset-clears-cache", func(t *testing.T) {
		ResetDynamicRegistryForTesting()
		if _, err := parseScope("json+schema"); err != nil {
			t.Fatal(err)
		}
		ResetDynamicRegistryForTesting()
		if got := cacheLen(); got != 0 {
			t.Fatalf("parsed-scope cache has %d entries after reset; want 0", got)
		}
	})
}
