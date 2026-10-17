package main

import "encoding/json"

// jsonUnmarshal is a tiny wrapper so main.go can parse the embedded dataset
// without importing encoding/json directly in multiple places.
func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
