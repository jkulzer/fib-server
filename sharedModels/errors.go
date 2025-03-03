package sharedModels

import (
	"errors"
)

var ErrHiderLocationNotInZone error = errors.New("Hider location must be in zone")
