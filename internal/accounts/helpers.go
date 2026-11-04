package accounts

import (
	"encoding/json"
	"os"
)

func isSSORoleCache(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var probe struct {
		ProviderType string `json:"ProviderType"`
	}
	return json.Unmarshal(b, &probe) == nil && probe.ProviderType == "sso"
}
