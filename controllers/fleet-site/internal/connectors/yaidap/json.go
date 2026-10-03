package yaidap

import "encoding/json"

func jsonDecode(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}
