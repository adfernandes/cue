// Note: this file is compiled on its own at the current language version by
// crdSchema, but "cue exp gengotypes" loads it as part of the cuelang.org/go
// CUE module, whose language version is older, so it names the experiments it
// uses. explicitopen is redundant from v0.18.0 on, where it is stable.
@experiment(try,explicitopen)

package jsonschema

// input holds the parsed YAML document, which may contain multiple
// Kubernetes resources, in which case it will be an array.
input!: _

// specs holds the resulting CRD specs: when there was only
// a single resource in the input, we'll get an array with one
// element.
// TODO(rog) replace comparisons to bottom with whatever the
// correct replacement will be.
specs: {
	[...]
	if (input & [...]) != _|_ {
		// It's an array: include only elements that look like CRDs.
		[
			for doc in input
			if (doc & {#crdlike...}) != _|_ {
				// Note: don't check for unification with #CRDSpec above because
				// we want it to fail if it doesn't unify with the entirety of #CRDSpec,
				// not just exclude the document.
				//
				// Both #crdlike and #CRDSpec are spread, as a CRD document holds
				// many fields these schemas do not describe, such as metadata.
				doc
				#CRDSpec...
			},
		]
	} else {
		// It's a single document. Include it if it looks like a CRD.
		// The schemas are spread here for the same reason as above.
		if (input & {#crdlike...}) != _|_ {
			[{input, #CRDSpec...}]
		}
	}
}

#crdlike: {
	apiVersion!: "apiextensions.k8s.io/v1"
	kind!:       "CustomResourceDefinition"
} @go(crdLike)

// CRDSpec defines a subset of the CRD schema, suitable for filtering
// CRDs based on common criteria like group and name.
#CRDSpec: {
	// #crdlike is spread so that it constrains the fields it declares
	// without closing this definition to just those fields.
	#crdlike...
	apiVersion!: "apiextensions.k8s.io/v1"
	kind!:       "CustomResourceDefinition"
	spec!: {
		group!: string
		names!: {
			kind!:     string
			plural!:   string
			singular!: string
		}
		scope!: "Namespaced" | "Cluster"
		versions!: [... {
			name!: string
			schema!: {
				openAPIV3Schema!: _ @go(,type="cuelang.org/go/cue".Value)
			}
		}]
	}
}
