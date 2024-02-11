package rdap

import (
	"encoding/json"
	"errors"
	"fmt"
)

type VCard []VCardProperty

type VCardProperty struct {
	Name   string
	Params map[string][]string
	Type   string
	Values []interface{}
}

func (pv *VCard) UnmarshalJSON(bs []byte) error {

	vc := make([]interface{}, 0, 2)
	e := json.Unmarshal(bs, &vc)
	if e != nil {
		return fmt.Errorf("vcardArray: %w", e)
	}

	if len(vc) < 2 {
		return errors.New("vcardArray: invalid -- not enough items in array")
	}

	if vc[0].(string) != "vcard" {
		return errors.New("vcardArray: invalid -- missing 'vcard' header")
	}

	props := vc[1].([]interface{})
	for pi := range props {

		arProp := props[pi].([]interface{})
		var vp VCardProperty

		for i := range arProp {
			switch i {
			case 0:
				vp.Name = arProp[i].(string)
			case 1:
				tmp := arProp[i].(map[string]interface{})
				vp.Params = make(map[string][]string, len(tmp))
				for tkey := range tmp {
					switch V := tmp[tkey].(type) {
					case []string:
						vp.Params[tkey] = V
					case string:
						vp.Params[tkey] = []string{V}
					}
				}
			case 2:
				vp.Type = arProp[i].(string)
			default:
				vp.Values = append(vp.Values, arProp[i])
			}
		}
		*pv = append(*pv, vp)
	}

	return nil
}
