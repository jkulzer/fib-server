package helpers

import (
	"io"
	"math"
	"math/rand"
	"os"

	"gorm.io/gorm"

	"github.com/rs/zerolog/log"

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

	f, err := os.Create("mapdata.geojson")
	defer f.Close()
	_, err = f.Write(areaJson)
	if err != nil {
		log.Err(err).Msg("")
	}

	result := db.Save(&lobby)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// normalizeBearing adjusts the angle to be within the range [-180, 180)
func NormalizeBearing(angle float64) float64 {
	// Shift the angle to the 0-360 range, then adjust back to -180-180
	angle = math.Mod(angle, 360)
	if angle > 180 {
		angle -= 360
	} else if angle < -180 {
		angle += 360
	}
	return angle
}
