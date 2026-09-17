package jsonx

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONBoundaries(t *testing.T) {
	for _, input := range []string{"{} {}", "{", string([]byte{'"', 255, '"'}), strings.Repeat("[", 129) + "0" + strings.Repeat("]", 129), `"` + strings.Repeat("x", MaxBytes) + `"`} {
		if _, err := Decode([]byte(input)); err == nil {
			t.Fatal("accepted invalid or oversized input")
		}
	}
	value, err := Decode([]byte(`{"id":1234567890123456789,"text":"[\\\"{}]"}`))
	if err != nil || value.(map[string]any)["id"] != json.Number("1234567890123456789") {
		t.Fatal(value, err)
	}
	for _, input := range []string{`{"key":"ENG-1","key":"ENG-2"}`, `{"fields":{"key":1,"\u006bey":2}}`} {
		if _, err := DecodeInput([]byte(input)); err == nil {
			t.Fatal("accepted duplicate key")
		}
	}
}

func FuzzDecode(f *testing.F) {
	for _, seed := range []string{`{"id":1}`, `{"fields":{"x":null}}`, `{"x":1,"\u0078":2}`, `[true,"text",{}]`, `{} {}`, `"\\\""`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 65536 {
			t.Skip()
		}
		value, err := DecodeInput([]byte(input))
		if err == nil {
			encoded, err := json.Marshal(value)
			if err != nil || !json.Valid(encoded) {
				t.Fatal("invalid decoded value")
			}
		}
	})
}
