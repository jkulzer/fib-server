package helpers

import (
	"io"
	"math/rand"
)

func ReadHttpResponse(input io.ReadCloser) ([]byte, error) {
	if b, err := io.ReadAll(input); err == nil {
		return b, err
	} else {
		return nil, err
	}
}

func ReadHttpResponseToString(input io.ReadCloser) (string, error) {
	if b, err := io.ReadAll(input); err == nil {
		return string(b), err
	} else {
		return "", err
	}
}
func RandomString(n int, charsetString string) string {
	letterRunes := []rune(charsetString)
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
