package content

import "encoding/json"

// MarshalJSON encodes nil warning slices as empty arrays.
func (item Item) MarshalJSON() ([]byte, error) {
	type itemJSON Item

	encoded := itemJSON(item)
	if encoded.Warnings == nil {
		encoded.Warnings = []Warning{}
	}

	return json.Marshal(encoded)
}

// MarshalJSON encodes nil warning slices as empty arrays.
func (pack Pack) MarshalJSON() ([]byte, error) {
	type packJSON Pack

	encoded := packJSON(pack)
	if encoded.Warnings == nil {
		encoded.Warnings = []Warning{}
	}

	return json.Marshal(encoded)
}
