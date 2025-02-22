package sharedModels

import (
	"time"
)

type CurrentDraw struct {
	Cards  []Card
	ToPick uint
}

type Card struct {
	Title              string
	Description        string
	Type               CardType
	ExpirationDuration time.Duration
	ActivationTime     time.Time
	BonusTime          time.Duration
}

func GetCardList() []Card {
	var cardList []Card
	cardList = append(cardList, []Card{
		{
			Title:       "Bridge curse",
			Description: "Next question must be asked under a bridge",
			Type:        CurseCard,
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
	PowerupCard
)
