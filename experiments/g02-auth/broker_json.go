package enrollment

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"
)

func decodeBrokerJSON(data []byte, target any, strict bool) error {
	tokens := json.NewDecoder(bytes.NewReader(data))
	if !uniqueBrokerJSON(tokens, 0) {
		return errBroker
	}
	if _, err := tokens.Token(); err != io.EOF {
		return errBroker
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if decoder.Decode(target) != nil {
		return errBroker
	}
	return nil
}
func uniqueBrokerJSON(d *json.Decoder, depth int) bool {
	if depth > 32 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return false
			}
			key, ok := token.(string)
			if !ok {
				return false
			}
			key = foldedBrokerName(key)
			if seen[key] || !uniqueBrokerJSON(d, depth+1) {
				return false
			}
			seen[key] = true
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for d.More() {
			if !uniqueBrokerJSON(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

// Match encoding/json's Unicode simple-fold field equivalence. These bounded
// decoders serve fixed authority/API schemas, not arbitrary case-distinct maps.
func foldedBrokerName(input string) string {
	var result strings.Builder
	result.Grow(len(input))
	for _, r := range input {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		result.WriteRune(minimum)
	}
	return result.String()
}
