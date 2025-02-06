package sharedModels

import (
	"github.com/google/uuid"
	"github.com/paulmach/orb"
	"time"
)

type LoginInfo struct {
	Username string
	Password string
}

type SessionToken struct {
	Token  uuid.UUID
	Expiry time.Time
}

type LobbyCreationResponse struct {
	LobbyToken string
}

type LobbyJoinRequest struct {
	LobbyToken string
}

type UserRole int

const (
	NoRole UserRole = iota
	Hider
	Seeker
)

type RoleAvailability []UserRole

var LobbyCodeRegex = "^[A-Z0-9]{6}$"

type UserRoleRequest struct {
	Role UserRole
}

type GamePhase int

const (
	PhaseBeforeStart GamePhase = iota
	PhaseRun
	PhaseLocationNarrowing
	PhaseEndgame
	PhaseFinished
	PhaseInvalid
)

type PhaseResponse struct {
	Phase GamePhase
}

type ReadinessResponse struct {
	Ready bool
}

type SetReadinessRequest struct {
	Ready bool
}

type TimeResponse struct {
	Time time.Time
}

// var RunDuration time.Duration = 45 * time.Minute

// var RunDuration time.Duration = 1 * time.Minute
var RunDuration time.Duration = 15 * time.Second

var HidingZoneRadius float64 = 500.0

type LocationRequest struct {
	Location orb.Point
}

var leftBound float64 = 12
var rightBound float64 = 15
var topBound float64 = 53
var bottomBound float64 = 51

func LeftTopPoint() orb.Point {
	var leftTopPoint orb.Point
	leftTopPoint[0] = leftBound
	leftTopPoint[1] = topBound
	return leftTopPoint
}

func RightTopPoint() orb.Point {
	var rightTopPoint orb.Point
	rightTopPoint[0] = rightBound
	rightTopPoint[1] = topBound
	return rightTopPoint
}

func RightBottomPoint() orb.Point {
	var rightBottomPoint orb.Point
	rightBottomPoint[0] = rightBound
	rightBottomPoint[1] = bottomBound
	return rightBottomPoint
}

func LeftBottomPoint() orb.Point {
	var leftBottomPoint orb.Point
	leftBottomPoint[0] = leftBound
	leftBottomPoint[1] = bottomBound
	return leftBottomPoint
}
