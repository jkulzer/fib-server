package helpers

import (
	"io"
)

func ReadHttpResponse(input io.ReadCloser) (string, error) {
	if b, err := io.ReadAll(input); err == nil {
		return string(b), err
	} else {
		return "", err
	}
}
