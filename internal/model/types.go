package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type JSONStringSlice []string

func (s JSONStringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *JSONStringSlice) Scan(src interface{}) error {
	var data []byte
	switch v := src.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	case nil:
		*s = JSONStringSlice{}
		return nil
	default:
		return fmt.Errorf("unsupported type for JSONStringSlice: %T", src)
	}
	return json.Unmarshal(data, s)
}
