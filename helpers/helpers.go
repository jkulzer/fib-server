package helpers

import (
	"gorm.io/gorm"
	"io"
	"math/rand"

	"github.com/jkulzer/fib-server/models"
	"github.com/paulmach/orb/geojson"
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

func FCFromDB(lobby models.Lobby) (*geojson.FeatureCollection, error) {
	excludedAreaBytes := []byte(lobby.ExcludedArea)
	fc, err := geojson.UnmarshalFeatureCollection(excludedAreaBytes)
	if err != nil {
		return &geojson.FeatureCollection{}, err
	}
	return fc, nil
}

func FCToDB(db *gorm.DB, lobby models.Lobby, fc *geojson.FeatureCollection) error {
	areaJson, err := fc.MarshalJSON()
	if err != nil {
		return err
	}
	lobby.ExcludedArea = string(areaJson)
	result := db.Save(&lobby)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
