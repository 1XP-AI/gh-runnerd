package liveworker

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

type dockerJSONObject uint8

const (
	dockerMapObject dockerJSONObject = iota
	dockerInspectObject
	dockerStateObject
	dockerNetworkObject
	dockerErrorObject
)

// Only names decoded into structs are folded by encoding/json. Config,
// HostConfig, Labels, network names and unknown objects remain case-sensitive
// maps. This prevents a broad folded-key check from rejecting valid labels.
func dockerStructNames(shape dockerJSONObject) []string {
	switch shape {
	case dockerInspectObject:
		return []string{"Path", "Args", "Id", "Name", "Image", "Config", "HostConfig", "Mounts", "State", "NetworkSettings"}
	case dockerStateObject:
		return []string{"Status", "Running", "Paused", "Restarting", "Dead", "ExitCode"}
	case dockerNetworkObject:
		return []string{"Networks"}
	case dockerErrorObject:
		return []string{"message"}
	}
	return nil
}

func validDockerJSON(data []byte, shape dockerJSONObject) bool {
	if !utf8.Valid(data) || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if !dockerJSONValue(d, shape, 0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

// Inspect bodies are byte-bounded by responseLimit; an additional nesting bound
// keeps recursive validation bounded even for unknown JSON objects/arrays.
func dockerJSONValue(d *json.Decoder, shape dockerJSONObject, depth int) bool {
	if depth > 64 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			key, ok := token.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			for _, canonical := range dockerStructNames(shape) {
				if key != canonical && foldedJSONName(key) == foldedJSONName(canonical) {
					return false
				}
			}
			child := dockerMapObject
			if shape == dockerInspectObject {
				switch key {
				case "State":
					child = dockerStateObject
				case "NetworkSettings":
					child = dockerNetworkObject
				}
			}
			if !dockerJSONValue(d, child, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for d.More() {
			if !dockerJSONValue(d, dockerMapObject, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	}
	return false
}
