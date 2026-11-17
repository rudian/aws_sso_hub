package taskstate

import "encoding/json"

func marshalReport(r Report) ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
