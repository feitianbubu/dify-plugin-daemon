package parser

import (
	"github.com/mitchellh/mapstructure"
)

func StructToMap(data interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	decoder := &mapstructure.DecoderConfig{
		Metadata: nil,
		Result:   &result,
		TagName:  "json",
		Squash:   true,
	}

	d, err := mapstructure.NewDecoder(decoder)
	if err != nil {
		return nil
	}

	err = d.Decode(data)
	if err != nil {
		return nil
	}

	return result
}
