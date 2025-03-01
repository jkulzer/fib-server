package sharedModels

import (
	"time"
)

type CurrentDraw struct {
	Cards  []Card
	ToPick uint
}

type Card struct {
	IDInDB                 uint
	Title                  string
	Description            string
	CastingCostDescription string
	Type                   CardType
	ExpirationDuration     time.Duration
	ActivationTime         time.Time
	BonusTime              time.Duration
}

func GetCardList() []Card {
	var cardList []Card
	cardList = append(cardList, []Card{
		{
			Title:       "Curse of the bridge troll",
			Description: "Next question must be asked under a bridge",
			Type:        CurseCard,
		},
		{
			Title:              "Curse of the jammed Door",
			Description:        "For the next 30mins, the seeker must roll a dice before passing a door. A dice roll can only be reattempted after 4 minutes. One can only roll for one door per building or train.",
			Type:               CurseCard,
			ExpirationDuration: time.Minute * 30,
		},
		{
			Title:                  "Curse of the mediocre travel agent",
			Description:            "The hider can send the seeker to a place 250m away from the seekers. They must go there and stay there for 5mins.",
			CastingCostDescription: "The place must be farther away from the hider that the seekers.",
			Type:                   CurseCard,
		},
		{
			Title:                  "Curse of the drained brain",
			Description:            "The hider can select three questions from three different categories that can't be asked for the remaining time of the run.",
			CastingCostDescription: "The hider must discard their entire hand.",
			Type:                   CurseCard,
		},
		{
			Title:                  "Curse of the zoologist",
			Description:            "The hider must take a picture of an animal and send it to the seeker (on a messaging app). The seeker must take a picture of an animal in the same class and send it to the hider. Until this is resolved, no questions may be asked.",
			CastingCostDescription: "Take a picture of a animal",
			Type:                   CurseCard,
		},
		{
			Title:                  "Curse of the right turn",
			Description:            "The seeker can only turn right or go straight for the next 20mins. If they end up in a dead end, a 180 turn is permissible.",
			ExpirationDuration:     time.Minute * 20,
			CastingCostDescription: "Discard two cards",
			Type:                   CurseCard,
		},
		{
			Title:                  "Curse of the census taker",
			Description:            "The seeker must estimate the population of the Bezirk they are in before they can ask more questions. If they guess the population within 25%, the curse clears. If they don't guess it within 25%, the curse gets cleared and the hider gets an extra 20 minutes",
			CastingCostDescription: "Discard one card", // originally it's the seekers next question is free, but that's difficult to implement. TODO for later
			Type:                   CurseCard,
		},
		{
			Title:                  "Curse of the urban explorer",
			Description:            "The seeker may only ask question outside of train stations and trains for the remainder of the run.",
			CastingCostDescription: "Discard two cards",
			Type:                   CurseCard,
		},
		{
			Title:                  "Curse of the bird guide",
			Description:            "The hider must take a video of a bird and send it to the seeker. The seeker must then take a video of a bird for longer than the seeker.",
			CastingCostDescription: "Take a video of a bird. The bird must be continously in frame for as long as possible",
			Type:                   CurseCard,
		},
	}...)
	for i := 0; i < 3; i++ {
		cardList = append(cardList,
			[]Card{
				{
					Title:     "5 Minute Bonus",
					Type:      TimebonusCard,
					BonusTime: time.Minute * 5,
				},
				{
					Title:     "10 Minute Bonus",
					Type:      TimebonusCard,
					BonusTime: time.Minute * 10,
				},
				{
					Title:     "15 Minute Bonus",
					Type:      TimebonusCard,
					BonusTime: time.Minute * 15,
				},
				{
					Title:     "30 Minute Bonus",
					Type:      TimebonusCard,
					BonusTime: time.Minute * 30,
				},
			}...)
	}

	return cardList
}

type CardType int

const (
	TimebonusCard CardType = iota
	CurseCard
	// Discard1Draw2Card
	// Discard2Draw3Card
)
