package enrollment

import (
	"bytes"
	"encoding/json"
	"io"
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
			if !ok || seen[key] || !uniqueBrokerJSON(d, depth+1) {
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
