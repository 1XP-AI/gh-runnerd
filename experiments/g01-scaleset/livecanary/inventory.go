package livecanary

import (
	"bytes"
	"encoding/json"
)

// inventoryPage keeps required-field presence local to Inventory. The shared
// HTTP reader retains its credential, destination, body and error behavior.
type inventoryPage struct {
	count int
	ids   []int64
}

func (p *inventoryPage) UnmarshalJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	// Unknown metadata is forward compatible, including numbers outside the
	// float64 range. It still cannot contain ambiguous object keys.
	d.UseNumber()
	if !uniqueKeys(d) {
		return ErrRemote
	}
	var wire struct {
		Count   *int `json:"total_count"`
		Runners *[]struct {
			ID *int64 `json:"id"`
		} `json:"runners"`
	}
	if json.Unmarshal(data, &wire) != nil || wire.Count == nil || wire.Runners == nil || *wire.Count < 0 || *wire.Count > 1000 || len(*wire.Runners) > 100 {
		return ErrRemote
	}
	var ids []int64
	for _, runner := range *wire.Runners {
		if runner.ID == nil || *runner.ID <= 0 {
			return ErrRemote
		}
		ids = append(ids, *runner.ID)
	}
	p.count, p.ids = *wire.Count, ids
	return nil
}
