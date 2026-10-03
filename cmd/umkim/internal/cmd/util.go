package cmd

import (
	"encoding/json"
	"os"
)

func readFileJSON(path string, dest interface{}) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}
