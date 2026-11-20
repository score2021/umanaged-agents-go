// NullOnly represents an OpenAPI property whose only JSON value is null.
package umanagedagents

import (
	"bytes"
	"fmt"
)

type NullOnly struct{}

func (NullOnly) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

func (*NullOnly) UnmarshalJSON(data []byte) error {
	if !bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("expected JSON null")
	}
	return nil
}
