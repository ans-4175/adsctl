// Package document reads strict, single-document YAML or JSON inputs.
package document

import (
	"adsctl/internal/output"
	"bytes"
	"errors"
	"go.yaml.in/yaml/v3"
	"io"
)

func Decode(b []byte, value any) error {
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(value); err != nil {
		return output.Fail("VALIDATION", "Invalid document or unknown fields: "+err.Error(), 5)
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return output.Fail("VALIDATION", "Expected one YAML/JSON document.", 5)
	}
	return nil
}
