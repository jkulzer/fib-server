package routes

import (
	"gorm.io/gorm"

	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/jkulzer/fib-server/controllers"
	"github.com/jkulzer/fib-server/geo"
	"github.com/jkulzer/fib-server/helpers"
	"github.com/jkulzer/fib-server/models"
	"github.com/jkulzer/fib-server/sharedModels"

	"github.com/jkulzer/osm"

	"github.com/paulmach/orb"
	orbGeo "github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"github.com/paulmach/orb/simplify"

	"github.com/engelsjk/polygol"

	chi "github.com/go-chi/chi/v5"

	"github.com/rs/zerolog/log"
)

func Router(r chi.Router, db *gorm.DB, processedData geo.ProcessedData) {
	r.Post("/register", func(w http.ResponseWriter, r *http.Request) {
		body, err := helpers.ReadHttpResponse(r.Body)
		if err != nil {
			log.Warn().Msg("failed to read http response")
		}

		var loginInfo sharedModels.LoginInfo
		err = json.Unmarshal(body, &loginInfo)
		if err != nil {
			log.Warn().Msg("failed to parse json of login info")
		} else {

			hashedPassword, err := controllers.HashPassword(loginInfo.Password)
			if err != nil {
				log.Warn().Msg("Failed to hash password")
			}

			userName := models.UserAccount{
				Name:     loginInfo.Username,
				Password: hashedPassword,
			}
			// tries to create the user in the db
			result := db.Create(&userName)

			// if the user creation fails,
			if result.Error != nil {
				log.Warn().Msg("Duplicate Username")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
			} else {
				w.WriteHeader(http.StatusCreated)
				w.Write(nil)
			}
		}
	},
	)
	r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		body, err := helpers.ReadHttpResponse(r.Body)
		if err != nil {
			log.Warn().Msg("failed to read http response")
		}

		var loginInfo sharedModels.LoginInfo
		err = json.Unmarshal(body, &loginInfo)
		if err != nil {
			log.Warn().Msg("failed to parse json of login info")
		} else {

			var userAccount models.UserAccount
			result := db.Where(&models.UserAccount{Name: loginInfo.Username}).First(&userAccount)

			if result.Error != nil {
				fmt.Println("Username not found")
			} else {

				// checks if password is correct
				if controllers.CheckPasswordHash(
					loginInfo.Password, userAccount.Password,
				) {
					token, expiry := controllers.NewSession(db, userAccount)
					jsonResponse, err := json.Marshal(sharedModels.SessionToken{
						Token:  token,
						Expiry: time.Now().Add(expiry),
					})
					if err != nil {
						log.Warn().Msg("failed to marshal response for sending session token")
					}
					w.WriteHeader(http.StatusCreated)
					w.Write(jsonResponse)
				} else {
					w.WriteHeader(http.StatusForbidden)
					w.Write(nil)
				}
			}

		}
	})
	r.Route("/lobby", func(r chi.Router) {
		r.Use(AuthMiddleware(db))
		r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
			userID, isUint := r.Context().Value(models.UserIDKey).(uint)

			if isUint {
				charsetString := "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"

				lobbyToken := helpers.RandomString(6, charsetString)

				var lobby models.Lobby

				// deletes all other lobbies owned by the creator of the current lobby
				// this ensures that no zombie lobbies exist in the database
				db.Where("creator_id = ?", userID).Delete(&models.Lobby{})
				lobby.Token = lobbyToken
				lobby.Phase = sharedModels.PhaseBeforeStart
				// lobby.CreatorID = userID
				log.Info().Msg("user ID " + fmt.Sprint(userID))
				lobby.CreatorID = userID
				newFC := geojson.NewFeatureCollection()

				var boundaryLineStrings []orb.LineString
				log.Debug().Msg("length of city boundary member list: " + fmt.Sprint(len(processedData.CityBoundary.Members)))
				for _, member := range processedData.CityBoundary.Members {
					if member.Type == "way" {
						wayID, err := member.ElementID().WayID()
						if err != nil {
							log.Err(err).Msg("")
							continue
						}
						way := processedData.Ways[wayID]
						lineString := geo.LineStringFromWay(way, processedData.Nodes)
						boundaryLineStrings = append(boundaryLineStrings, lineString)
					}
				}
				log.Debug().Msg("done passing through city boundary members")
				boundaryFromLS, err := geo.RingFromLineStrings(boundaryLineStrings)
				if err != nil {
					log.Err(err).Msg("")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				boundaryFromLS.Reverse()
				berlinBoundary := orb.Polygon([]orb.Ring{sharedModels.WideOutsideBound(), boundaryFromLS})

				berlinBoundary[0].Reverse()
				berlinBoundary = simplify.VisvalingamKeep(1500).Polygon(berlinBoundary)
				newFC.Append(geojson.NewFeature(berlinBoundary))
				log.Debug().Msg("appended boundary")

				marshalledJSON, err := newFC.MarshalJSON()
				if err != nil {
					log.Err(err).Msg("failed marshalling new empty FC JSON")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				lobby.ExcludedArea = string(marshalledJSON)
				result := db.Create(&lobby)
				if result.Error != nil {
					log.Err(result.Error).Msg("failed to create lobby in database")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				lobbyCreationResponse := sharedModels.LobbyCreationResponse{
					LobbyToken: lobbyToken,
				}

				helpers.FCToDB(db, lobby, newFC)

				fmt.Println("Created lobby with token " + lobbyToken)
				marshalledJson, err := json.Marshal(lobbyCreationResponse)
				if err != nil {
					log.Err(err).Msg("failed to marshal lobby creation response")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
				} else {
					w.WriteHeader(http.StatusCreated)
					w.Write(marshalledJson)
				}
			} else {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
			}
		},
		)
		r.Post("/join", func(w http.ResponseWriter, r *http.Request) {
			body, err := helpers.ReadHttpResponse(r.Body)
			if err != nil {
				log.Err(err).Msg("failed to read http response")
			}

			var lobbyJoinRequest sharedModels.LobbyJoinRequest
			err = json.Unmarshal(body, &lobbyJoinRequest)
			if err != nil {
				log.Warn().Msg("failed to parse json of lobby join request")
			} else {

				lobby := models.Lobby{
					Token: lobbyJoinRequest.LobbyToken,
				}

				result := db.First(&lobby)
				if result.Error != nil {
					w.WriteHeader(http.StatusNotFound)
					w.Write(nil)
				} else {
					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				}
			}
		},
		)
		r.Route("/{index}", func(r chi.Router) {
			r.Use(LobbyMiddleware(db))
			r.Get("/map", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(lobby.ExcludedArea))
			})
			r.Get("/phase", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				phaseResponse := sharedModels.PhaseResponse{
					Phase: lobby.Phase,
				}

				marshalledJson, err := json.Marshal(phaseResponse)
				if err != nil {
					log.Err(err).Msg("failed to marshal phase status")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(marshalledJson)
			})
			r.Get("/readiness", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				var readiness sharedModels.ReadinessResponse
				if lobby.HiderReady && lobby.SeekerReady {
					readiness.Ready = true
				} else {
					readiness.Ready = false
				}

				marshalledJson, err := json.Marshal(readiness)
				if err != nil {
					log.Err(err).Msg("failed to marshal readiness response")
				}
				w.WriteHeader(http.StatusOK)
				w.Write(marshalledJson)
			})
			r.Put("/saveLocation", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if isUint == false {
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				body, err := helpers.ReadHttpResponse(r.Body)
				if err != nil {
					log.Err(err).Msg("failed to read http request of body " + fmt.Sprint(err))
				}

				var locationRequest sharedModels.LocationRequest
				err = json.Unmarshal(body, &locationRequest)
				if err != nil {
					log.Warn().Msg("failed to parse json of setting location")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
				}
				switch userID {
				case lobby.SeekerID:
					// yes, longitude comes first, look at https://pkg.go.dev/github.com/paulmach/orb#Point
					lobby.SeekerLat = locationRequest.Location[1]
					lobby.SeekerLon = locationRequest.Location[0]
				case lobby.HiderID:
					// yes, longitude comes first, look at https://pkg.go.dev/github.com/paulmach/orb#Point
					lobby.HiderLat = locationRequest.Location[1]
					lobby.HiderLon = locationRequest.Location[0]
				default:
					w.WriteHeader(http.StatusForbidden)
					w.Write(nil)
					return
				}

				result := db.Save(&lobby)
				if result.Error != nil {
					log.Err(err).Msg("failed to save location to DB with error " + fmt.Sprint(result.Error))
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write(nil)
			})
			r.Put("/saveHidingZone", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if isUint == false {
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				body, err := helpers.ReadHttpResponse(r.Body)
				if err != nil {
					log.Err(err).Msg("failed to read http request of body " + fmt.Sprint(err))
				}

				var locationRequest sharedModels.LocationRequest
				err = json.Unmarshal(body, &locationRequest)
				if err != nil {
					log.Warn().Msg("failed to parse json of setting hiding spot")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
				}
				if userID != lobby.HiderID {
					w.WriteHeader(http.StatusForbidden)
					w.Write(nil)
					return
				}

				isValidPoint := geo.PointIsValidZoneCenter(locationRequest.Location, processedData)

				if !isValidPoint {
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
					return
				}

				// yes, longitude comes first, look at https://pkg.go.dev/github.com/paulmach/orb#Point
				lobby.ZoneCenterLat = locationRequest.Location[1]
				lobby.ZoneCenterLon = locationRequest.Location[0]
				// also initialize user location
				lobby.HiderLat = locationRequest.Location[1]
				lobby.HiderLon = locationRequest.Location[0]

				log.Debug().Msg(fmt.Sprint("saved zone center", locationRequest.Location))

				result := db.Save(&lobby)
				if result.Error != nil {
					log.Err(err).Msg("failed to save readiness info to DB  with error " + fmt.Sprint(result.Error))
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write(nil)
			})
			r.Put("/readiness", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if isUint == false {
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				body, err := helpers.ReadHttpResponse(r.Body)
				if err != nil {
					log.Err(err).Msg("failed to read http request of body " + fmt.Sprint(err))
				}
				var readinessRequest sharedModels.SetReadinessRequest
				err = json.Unmarshal(body, &readinessRequest)
				if err != nil {
					log.Warn().Msg("failed to parse json of readiness set request")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
				}

				switch userID {
				case lobby.HiderID:
					lobby.HiderReady = readinessRequest.Ready
				case lobby.SeekerID:
					lobby.SeekerReady = readinessRequest.Ready
				default:
					log.Warn().Msg("user made reqest to set readiness for lobby " + fmt.Sprint(lobby.Token) + " and isn't hider or seeker")
					w.WriteHeader(http.StatusBadRequest)
				}

				result := db.Save(&lobby)
				if result.Error != nil {
					log.Err(err).Msg("failed to save readiness info to DB  with error " + fmt.Sprint(result.Error))
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				if lobby.HiderReady && lobby.SeekerReady {
					lobby.Phase = sharedModels.PhaseRun
					lobby.RunStartTime = time.Now()
					result = db.Save(&lobby)
					if result.Error != nil {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}
					go func() {
						runTimer := time.NewTimer(sharedModels.RunDuration)
						lobbyToken := lobby.Token

						<-runTimer.C
						log.Info().Msg("Hiding Time Finished")
						var lobby models.Lobby
						result := db.Where("token = ?", lobbyToken).First(&lobby)
						if result.Error != nil {
							log.Err(err).Msg("failed to save finished hiding time to DB for lobby " + lobbyToken)
							return
						}
						lobby.Phase = sharedModels.PhaseLocationNarrowing
						result = db.Save(&lobby)
						if result.Error != nil {
							log.Err(result.Error).Msg("")
						}
					}()
				}

				w.WriteHeader(http.StatusOK)
				w.Write(nil)
			})
			r.Get("/runStartTime", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				var startTime sharedModels.TimeResponse

				startTime.Time = lobby.RunStartTime

				marshalledJson, err := json.Marshal(startTime)
				if err != nil {
					log.Err(err).Msg("failed to marshal roles get")
				}
				w.WriteHeader(http.StatusOK)
				w.Write(marshalledJson)
			})
			r.Get("/roles", func(w http.ResponseWriter, r *http.Request) {
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				var roleAvailability []sharedModels.UserRole
				// both hider and seeker are available
				if lobby.SeekerID == 0 && lobby.HiderID == 0 {
					roleAvailability = []sharedModels.UserRole{sharedModels.Seeker, sharedModels.Hider}
					// only seeker is available
				} else if lobby.SeekerID == 0 && lobby.HiderID != 0 {
					roleAvailability = []sharedModels.UserRole{sharedModels.Seeker}
					// only hider is available
				} else if lobby.SeekerID != 0 && lobby.HiderID == 0 {
					roleAvailability = []sharedModels.UserRole{sharedModels.Hider}
					// nothing available
				} else {
					roleAvailability = []sharedModels.UserRole{}
				}

				marshalledJson, err := json.Marshal(roleAvailability)
				if err != nil {
					log.Err(err).Msg("failed to marshal roles get")
				}
				w.WriteHeader(http.StatusOK)
				w.Write(marshalledJson)
			})
			r.Post("/selectRole", func(w http.ResponseWriter, r *http.Request) {
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if !isUint {
					log.Debug().Msg(fmt.Sprint(userID))
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				var roleRequest sharedModels.UserRoleRequest
				body, err := helpers.ReadHttpResponse(r.Body)
				if err != nil {
					log.Err(err).Msg("failed to read http request when assigning user to role")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
					return
				}
				err = json.Unmarshal(body, &roleRequest)
				if err != nil {
					log.Warn().Msg("failed to parse json of user role assignment request")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
					return
				}
				log.Info().Msg("user with ID " + fmt.Sprint(userID) + " selected role " + fmt.Sprint(roleRequest.Role))
				if roleRequest.Role == sharedModels.Hider {
					if lobby.HiderID == 0 || lobby.HiderID == userID {
						lobby.HiderID = userID
						result := db.Save(&lobby)
						if result.Error != nil {
							log.Err(err).Msg(fmt.Sprint(err))
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						w.WriteHeader(http.StatusOK)
						w.Write(nil)
					} else {
						w.WriteHeader(http.StatusConflict)
						w.Write(nil)
						return
					}
				} else if roleRequest.Role == sharedModels.Seeker {
					if lobby.SeekerID == 0 || lobby.SeekerID == userID {
						lobby.SeekerID = userID
						result := db.Save(&lobby)
						if result.Error != nil {
							log.Err(err).Msg(fmt.Sprint(err))
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						w.WriteHeader(http.StatusOK)
						w.Write(nil)
					} else {
						w.WriteHeader(http.StatusConflict)
						w.Write(nil)
						return
					}
				}
				result := db.Save(&lobby)
				if result.Error != nil {
					log.Warn().Msg("failed to save role in db")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
			})
			r.Get("/cardActions", func(w http.ResponseWriter, r *http.Request) {
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if !isUint {
					log.Debug().Msg(fmt.Sprint(userID))
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				var cardDraws []sharedModels.CardDraw
				for _, cardDraw := range lobby.CardDraws {
					cardDraws = append(cardDraws, sharedModels.CardDraw{
						DrawID:      cardDraw.ID,
						CardsToDraw: cardDraw.CardsToDraw,
						CardsToPick: cardDraw.CardsToPick,
					})
				}

				marshaledResponse, err := json.Marshal(sharedModels.CardDraws{
					LobbyID: lobby.ID,
					Draws:   cardDraws,
				})
				if err != nil {
					log.Err(err).Msg("failed marshaling history json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
				}

				w.WriteHeader(http.StatusOK)
				w.Write(marshaledResponse)
			})
			r.Post("/drawCards/{drawID}", func(w http.ResponseWriter, r *http.Request) {
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if !isUint {
					log.Debug().Msg(fmt.Sprint(userID))
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				drawIDString := chi.URLParam(r, "drawID")

				drawID, err := strconv.ParseUint(drawIDString, 10, 64)
				if err != nil {
					log.Err(err).Msg("failed to parse draw ID in url")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
				}

				var draw models.CardDraw

				result := db.First(&draw, drawID)
				if result.Error != nil {
					log.Err(result.Error).Msg("couldn't find card draws")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				if draw.LobbyID != lobby.ID {
					log.Warn().Msg("card draw doesn't have matching lobby ID " + fmt.Sprint(draw.LobbyID, lobby.ID))
					w.WriteHeader(http.StatusNotFound)
					w.Write(nil)
					return
				}

				db.Preload("CurrentDraw").Find(&lobby)
				db.Preload("CurrentDraw.Cards").Find(&lobby)
				if len(lobby.CurrentDraw.Cards) != 0 {
					log.Err(err).Msg("card draw already in progress")
					w.WriteHeader(http.StatusConflict)
					w.Write(nil)
					return
				}

				for i := uint(1); i <= draw.CardsToDraw; i++ {
					log.Debug().Msg("getting random card: i=" + fmt.Sprint(i) + " limit is  " + fmt.Sprint(draw.CardsToDraw))

					if len(lobby.RemainingCards) < 1 {
						cardList := sharedModels.GetCardList()
						for _, card := range cardList {
							lobby.RemainingCards = append(lobby.RemainingCards, helpers.ExternalToInternalCard(card))
						}
					}

					randomCardIndex := rand.Intn(len(lobby.RemainingCards))

					generatedCard := lobby.RemainingCards[randomCardIndex]
					lobby.CurrentDraw.Cards = append(lobby.CurrentDraw.Cards, models.Card{
						Title:              generatedCard.Title,
						Description:        generatedCard.Description,
						Type:               generatedCard.Type,
						ExpirationDuration: generatedCard.ExpirationDuration,
						ActivationTime:     generatedCard.ActivationTime,
						BonusTime:          generatedCard.BonusTime,
					})

					lobby.RemainingCards = slices.Delete(lobby.RemainingCards, randomCardIndex, randomCardIndex)
				}
				lobby.CurrentDraw.ToPick = draw.CardsToPick

				result = db.Save(&lobby)
				if result.Error != nil {
					log.Err(result.Error).Msg("failed saving lobby")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(nil)
			})
			r.Get("/draw", func(w http.ResponseWriter, r *http.Request) {
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if !isUint {
					log.Debug().Msg(fmt.Sprint(userID))
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				db.Preload("CurrentDraw").Find(&lobby)
				db.Preload("CurrentDraw.Cards").Find(&lobby)

				fmt.Println(lobby.CurrentDraw)

				marshaledResponse, err := json.Marshal(lobby.CurrentDraw)
				if err != nil {
					log.Err(err).Msg("failed marshaling current draw")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
				}
				w.WriteHeader(http.StatusOK)
				w.Write(marshaledResponse)
			})
			r.Get("/history", func(w http.ResponseWriter, r *http.Request) {
				userID, isUint := r.Context().Value(models.UserIDKey).(uint)
				if !isUint {
					log.Debug().Msg(fmt.Sprint(userID))
					log.Warn().Msg("failed to convert userID to uint in role selection")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
				if !isLobby {
					fmt.Println(lobby)
					log.Warn().Msg("couldn't cast lobby value from context")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				var historyList []sharedModels.HistoryItem
				for _, dbItem := range lobby.History {
					historyList = append(historyList, sharedModels.HistoryItem{
						Title:       dbItem.Title,
						Description: dbItem.Description,
					})
				}

				marshaledHistory, err := json.Marshal(historyList)
				if err != nil {
					log.Err(err).Msg("failed marshaling history json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
				}

				w.WriteHeader(http.StatusOK)
				w.Write(marshaledHistory)
			})
			r.Route("/questions", func(r chi.Router) {
				r.Use(AuthMiddleware(db))
				r.Get("/closeRoutes", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					var seekerPoint orb.Point
					seekerPoint[0] = lobby.SeekerLon
					seekerPoint[1] = lobby.SeekerLat

					closeRoutes := make(map[osm.RelationID]*osm.Relation)

				routeIteration:
					for routeID, route := range processedData.AllRailRoutes {
					memberIteration:
						for _, member := range route.Members {
							if member.Type == "way" {
								memberWayID, err := member.ElementID().WayID()
								if err != nil {
									log.Err(err).Msg("element id: " + fmt.Sprint(member.ElementID()))
									continue memberIteration
								}
								memberWay := processedData.Ways[memberWayID]
								lineString := geo.LineStringFromWay(memberWay, processedData.Nodes)
								for _, routePoint := range lineString {
									if orbGeo.DistanceHaversine(routePoint, seekerPoint) < 300 {
										closeRoutes[routeID] = route
										continue routeIteration
									}
								}
							}
						}
					}
					response := sharedModels.RouteProximityResponse{}
					for _, route := range closeRoutes {
						routeItem := sharedModels.RouteDetails{
							Name:    route.Tags.Find("ref"),
							RouteID: route.ID,
						}
						response.Routes = append(response.Routes, routeItem)
					}

					marshaledReponse, err := json.Marshal(response)
					if err != nil {
						log.Err(err).Msg("failed to marshal route proximity reponse")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(marshaledReponse)
				})
				r.Post("/trainService", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					body, err := helpers.ReadHttpResponse(r.Body)
					if err != nil {
						log.Err(err).Msg("failed to read http request of body " + fmt.Sprint(err))
					}

					var trainServiceRequest sharedModels.TrainServiceRequest
					err = json.Unmarshal(body, &trainServiceRequest)
					if err != nil {
						log.Warn().Msg("failed to parse json of train service request")
						w.WriteHeader(http.StatusBadRequest)
						w.Write(nil)
					}

					var zoneCenter orb.Point
					zoneCenter[1] = lobby.ZoneCenterLat
					zoneCenter[0] = lobby.ZoneCenterLon

					route := processedData.Relations[trainServiceRequest.RouteID]

					fc, err := helpers.FCFromDB(lobby)

					if err != nil {
						log.Err(err).Msg("")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					isOnLine := false

				memberIteration:
					for _, member := range route.Members {
						if member.Type == "node" {
							memberNodeID, err := member.ElementID().NodeID()
							if err != nil {
								log.Err(err).Msg("element id: " + fmt.Sprint(member.ElementID()))
								continue memberIteration
							}
							memberNode := processedData.Nodes[memberNodeID]
							if memberNode == nil {
								continue
							}
							if memberNode.Tags.Find("railway") != "stop" {
								continue memberIteration
							}
							stopPositionPoint := helpers.NodeToPoint(*memberNode)
							if orbGeo.DistanceHaversine(stopPositionPoint, zoneCenter) <= sharedModels.HidingZoneRadius {
								log.Debug().Msg("hider is at stop " + memberNode.Tags.Find("name") + " with ID " + fmt.Sprint(memberNode.ElementID()))
								isOnLine = true
							}
						}
					}

					var circleGeomList []polygol.Geom
					var circleList []orb.Ring

				secondMemberIteration:
					for _, member := range route.Members {
						if member.Type == "node" {
							memberNodeID, err := member.ElementID().NodeID()
							if err != nil {
								log.Err(err).Msg("element id: " + fmt.Sprint(member.ElementID()))
								continue secondMemberIteration
							}
							memberNode := processedData.Nodes[memberNodeID]
							if memberNode == nil {
								continue secondMemberIteration
							}
							if memberNode.Tags.Find("railway") != "stop" {
								continue secondMemberIteration
							}
							stopPositionPoint := helpers.NodeToPoint(*memberNode)
							var circle orb.Ring
							if isOnLine {
								circle = helpers.NewCircle(stopPositionPoint, sharedModels.HidingZoneRadius*2)
							} else {
								circle = helpers.NewCircle(stopPositionPoint, sharedModels.HidingZoneRadius)
							}

							circleList = append(circleList, circle)
							geomCircle := helpers.G2p(orb.Polygon{circle})
							circleGeomList = append(circleGeomList, geomCircle)
						}
					}

					var description string

					if isOnLine {
						outsideGeom := helpers.G2p(orb.Polygon{sharedModels.WideOutsideBound()})
						diff, err := polygol.Difference(outsideGeom, circleGeomList...)
						if err != nil {
							log.Err(err).Msg("failed to make union of circles for train service question")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}
						exclusionPolygons := helpers.P2g(diff)

						for index := range exclusionPolygons {
							if index < len(exclusionPolygons)-1 {
								exclusionPolygons[index] = append(exclusionPolygons[index], sharedModels.WideOutsideBound())
							}
						}
						fc.Append(geojson.NewFeature(exclusionPolygons))
						description = route.Tags.Find("name") + " stops in the hiding zone"
					} else {
						for _, circle := range circleList {
							fc.Append(geojson.NewFeature(circle))
						}
						description = route.Tags.Find("name") + " doesn't stop in the hiding zone"
					}

					historyItem := models.HistoryInDB{
						LobbyID:     lobby.ID,
						Title:       "Train Service",
						Description: description,
					}
					result := db.Create(&historyItem)
					if result.Error != nil {
						log.Err(err).Msg("failed creating history item")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.CreateCardDraw(db, 3, 1, lobby.ID, w)
					if err != nil {
						log.Err(result.Error).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.FCToDB(db, lobby, fc)
					if err != nil {
						log.Err(err).Msg("failed to save fc to db during train service question request")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				})
				r.Post("/radar/{radius}", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					radarRadiusString := chi.URLParam(r, "radius")

					radius, err := strconv.ParseFloat(radarRadiusString, 64)
					if err != nil {
						log.Err(err).Msg("failed parsing radar radius")
						w.WriteHeader(http.StatusBadRequest)
						w.Write(nil)
						return
					}

					var hiderPoint orb.Point
					hiderPoint[0] = lobby.HiderLon
					hiderPoint[1] = lobby.HiderLat
					var seekerPoint orb.Point
					seekerPoint[0] = lobby.SeekerLon
					seekerPoint[1] = lobby.SeekerLat

					distanceSeekerHider := orbGeo.DistanceHaversine(hiderPoint, seekerPoint)
					seekerAddr, err := getClosestAdressString(seekerPoint)
					if err != nil {
						log.Err(err).Msg("failed getting seeker address")
						w.WriteHeader(http.StatusBadRequest)
						w.Write(nil)
						return
					}

					var radiusDistance string

					if radius < 1000 {
						radiusDistance = fmt.Sprint(radius) + "m"
					} else {
						radiusDistance = fmt.Sprint(radius/1000) + "km"
					}
					fc, err := helpers.FCFromDB(lobby)
					if err != nil {
						log.Err(err).Msg("")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					if distanceSeekerHider < radius {
						// it's a hit!
						log.Debug().Msg("it's a match")
						inverseCircle := helpers.NewInverseCircle(seekerPoint, radius)
						inverseCircleFeature := geojson.NewFeature(inverseCircle)
						fc.Append(inverseCircleFeature)
						historyItem := models.HistoryInDB{
							LobbyID:     lobby.ID,
							Title:       "Radar",
							Description: "Hider is within " + radiusDistance + " of " + seekerAddr,
						}
						result := db.Create(&historyItem)
						if result.Error != nil {
							log.Err(err).Msg("failed creating history item")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}
					} else {
						// it's a miss!
						log.Debug().Msg("it's not a match")
						circle := helpers.NewCircle(seekerPoint, radius)
						circleFeature := geojson.NewFeature(circle)
						fc, err := helpers.FCFromDB(lobby)
						fc.Append(circleFeature)
						if err != nil {
							log.Err(err).Msg("")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						historyItem := models.HistoryInDB{
							LobbyID:     lobby.ID,
							Title:       "Radar",
							Description: "Hider is not within " + radiusDistance + " of " + seekerAddr,
						}
						result := db.Create(&historyItem)
						if result.Error != nil {
							log.Err(err).Msg("failed creating history item")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}
					}
					err = helpers.FCToDB(db, lobby, fc)
					if err != nil {
						log.Err(err).Msg("")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}
					err = helpers.CreateCardDraw(db, 2, 1, lobby.ID, w)
					if err != nil {
						log.Err(err).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

				})
				r.Route("/thermometer", func(r chi.Router) {
					r.Post("/start", func(w http.ResponseWriter, r *http.Request) {
						lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
						if !isLobby {
							fmt.Println(lobby)
							log.Warn().Msg("couldn't cast lobby value from context")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						body, err := helpers.ReadHttpResponse(r.Body)
						if err != nil {
							log.Err(err).Msg("failed to read http request of body " + fmt.Sprint(err))
						}

						if lobby.ThermometerDistance != 0 {
							log.Info().Msg("thermometer already started! can't start another one")
							w.WriteHeader(http.StatusConflict)
							w.Write(nil)
							return
						}

						var thermometerRequest sharedModels.ThermometerRequest
						err = json.Unmarshal(body, &thermometerRequest)
						if err != nil {
							log.Err(err).Msg("")
							w.WriteHeader(http.StatusBadRequest)
							w.Write(nil)
							return
						}
						lobby.ThermometerStartLon = lobby.SeekerLon
						lobby.ThermometerStartLat = lobby.SeekerLat
						lobby.ThermometerDistance = thermometerRequest.Distance

						result := db.Save(&lobby)
						if result.Error != nil {
							log.Err(result.Error).Msg("")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}
						w.WriteHeader(http.StatusOK)
						w.Write(nil)
					})
					r.Post("/end", func(w http.ResponseWriter, r *http.Request) {
						lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
						if !isLobby {
							fmt.Println(lobby)
							log.Warn().Msg("couldn't cast lobby value from context")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						if lobby.ThermometerDistance == 0 {
							w.WriteHeader(http.StatusBadRequest)
							w.Write(nil)
							return
						}

						var hiderPoint orb.Point
						hiderPoint[0] = lobby.HiderLon
						hiderPoint[1] = lobby.HiderLat

						var seekerPoint orb.Point
						seekerPoint[0] = lobby.SeekerLon
						seekerPoint[1] = lobby.SeekerLat

						var thermometerStartPoint orb.Point
						thermometerStartPoint[0] = lobby.ThermometerStartLon
						thermometerStartPoint[1] = lobby.ThermometerStartLat

						if orbGeo.DistanceHaversine(thermometerStartPoint, seekerPoint) < lobby.ThermometerDistance {
							w.WriteHeader(http.StatusMethodNotAllowed)
							w.Write(nil)
							return
						}

						var description string

						thermometerBearing := orbGeo.Bearing(thermometerStartPoint, seekerPoint)
						var leftBearing float64
						var rightBearing float64

						if thermometerBearing < -90 {
							leftBearing = thermometerBearing + 90
							rightBearing = thermometerBearing + 270
						} else if thermometerBearing > 90 {
							leftBearing = thermometerBearing - 270
							rightBearing = thermometerBearing - 90
						} else {
							leftBearing = thermometerBearing + 90
							rightBearing = thermometerBearing - 90
						}

						thermometerStartAddr, err := getClosestAdressString(thermometerStartPoint)
						if err != nil {
							log.Err(err).Msg("failed getting address string")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}
						thermometerEndAddr, err := getClosestAdressString(seekerPoint)
						if err != nil {
							log.Err(err).Msg("failed getting address string")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						// if thermometer is hotter
						if orbGeo.DistanceHaversine(seekerPoint, hiderPoint) < orbGeo.DistanceHaversine(thermometerStartPoint, hiderPoint) {
							if thermometerBearing > 0 {
								thermometerBearing = thermometerBearing - 180
							} else {
								thermometerBearing = thermometerBearing + 180
							}
							description = "Hider is closer to " + fmt.Sprint(thermometerEndAddr) + " then to " + fmt.Sprint(thermometerStartAddr)
						} else {
							description = "Hider is closer to " + fmt.Sprint(thermometerStartAddr) + " then to " + fmt.Sprint(thermometerEndAddr)
						}

						boxFrontLeft := orbGeo.PointAtBearingAndDistance(orbGeo.PointAtBearingAndDistance(seekerPoint, thermometerBearing, 30000), leftBearing, 30000)
						boxFrontRight := orbGeo.PointAtBearingAndDistance(orbGeo.PointAtBearingAndDistance(seekerPoint, thermometerBearing, 30000), rightBearing, 30000)
						boxLeft := orbGeo.PointAtBearingAndDistance(seekerPoint, leftBearing, 30000)
						boxRight := orbGeo.PointAtBearingAndDistance(seekerPoint, rightBearing, 30000)

						boxPolygon := orb.Polygon{orb.Ring{boxFrontLeft, boxFrontRight, boxRight, boxLeft, boxFrontLeft}}

						fc, err := helpers.FCFromDB(lobby)
						if err != nil {
							log.Err(err).Msg("")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}
						fc.Append(geojson.NewFeature(boxPolygon))
						lobby.ThermometerDistance = 0

						historyItem := models.HistoryInDB{
							LobbyID:     lobby.ID,
							Title:       "Thermometer",
							Description: description,
						}
						result := db.Create(&historyItem)
						if result.Error != nil {
							log.Err(err).Msg("failed creating history item")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						err = helpers.CreateCardDraw(db, 2, 1, lobby.ID, w)
						if err != nil {
							log.Err(result.Error).Msg("failed creating card draw")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						err = helpers.FCToDB(db, lobby, fc)
						if err != nil {
							log.Err(err).Msg("")
							w.WriteHeader(http.StatusInternalServerError)
							w.Write(nil)
							return
						}

						w.WriteHeader(http.StatusOK)
						w.Write(nil)
					})
				})
				r.Post("/sameBezirk", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					fc, err := helpers.FCFromDB(lobby)
					if err != nil {
						log.Err(err).Msg("failed to get FC from DB while asking same bezirk question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					var zoneCenter orb.Point
					zoneCenter[1] = lobby.ZoneCenterLat
					zoneCenter[0] = lobby.ZoneCenterLon

					var seekerPoint orb.Point
					seekerPoint[1] = lobby.SeekerLat
					seekerPoint[0] = lobby.SeekerLon

					var hiderPoint orb.Point
					hiderPoint[1] = lobby.HiderLat
					hiderPoint[0] = lobby.HiderLon

					polygonMap := make(map[string]orb.MultiPolygon)

					for _, relation := range processedData.Bezirke {
						bezirkName := relation.Tags.Find("name")
						if bezirkName == "" {
							log.Warn().Msg("bezirk with ID " + fmt.Sprint(relation.ElementID()) + " has empty name field")
							continue
						}
						multiPolygon, err := geo.RelationToMultiPolygon(*relation, processedData.Nodes, processedData.Ways)
						if err != nil {
							log.Err(err).Msg("failed converting relation to polygon")
							continue
						}
						polygonMap[bezirkName] = multiPolygon
					}

					var seekerBezirk string
					var hiderBezirk string

					for bezirkName, multiPolygon := range polygonMap {
						if planar.MultiPolygonContains(multiPolygon, seekerPoint) {
							seekerBezirk = bezirkName
						}
						if planar.MultiPolygonContains(multiPolygon, hiderPoint) {
							hiderBezirk = bezirkName
						}
					}

					var description string

					if hiderBezirk == seekerBezirk {
						description = "Hider is in " + fmt.Sprint(hiderBezirk)
						for bezirkName, polygon := range polygonMap {
							if bezirkName != hiderBezirk {
								fc.Append(geojson.NewFeature(polygon))
							}
						}
					} else {
						description = "Hider is not in " + fmt.Sprint(seekerBezirk)
						for bezirkName, polygon := range polygonMap {
							if bezirkName == seekerBezirk {
								fc.Append(geojson.NewFeature(polygon))
							}
						}
					}
					historyItem := models.HistoryInDB{
						LobbyID:     lobby.ID,
						Title:       "Same Bezirk",
						Description: description,
					}
					result := db.Create(&historyItem)
					if result.Error != nil {
						log.Err(err).Msg("failed creating history item")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					log.Debug().Msg("seeker bezirk is " + seekerBezirk + " and hider bezirk is " + hiderBezirk)

					err = helpers.CreateCardDraw(db, 3, 1, lobby.ID, w)
					if err != nil {
						log.Err(result.Error).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.FCToDB(db, lobby, fc)
					if err != nil {
						log.Err(err).Msg("failed to save FC to DB while asking same bezirk question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				})
				r.Post("/sameOrtsteil", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					fc, err := helpers.FCFromDB(lobby)
					if err != nil {
						log.Err(err).Msg("failed to get FC from DB while asking same bezirk question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					var zoneCenter orb.Point
					zoneCenter[1] = lobby.ZoneCenterLat
					zoneCenter[0] = lobby.ZoneCenterLon

					var seekerPoint orb.Point
					seekerPoint[1] = lobby.SeekerLat
					seekerPoint[0] = lobby.SeekerLon

					var hiderPoint orb.Point
					hiderPoint[1] = lobby.HiderLat
					hiderPoint[0] = lobby.HiderLon

					multiPolygonMap := make(map[string]orb.MultiPolygon)

					for _, relation := range processedData.Ortsteile {
						ortsteilName := relation.Tags.Find("name")
						if ortsteilName == "" {
							log.Warn().Msg("ortsteil with ID " + fmt.Sprint(relation.ElementID()) + " has empty name field")
							continue
						}
						multiPolygon, err := geo.RelationToMultiPolygon(*relation, processedData.Nodes, processedData.Ways)
						if err != nil {
							log.Err(err).Msg("failed converting relation to polygon")
							continue
						}
						multiPolygonMap[ortsteilName] = multiPolygon
					}

					var seekerOrtsteil string
					var hiderOrtsteil string

					for ortsteilName, multiPolygon := range multiPolygonMap {
						if planar.MultiPolygonContains(multiPolygon, seekerPoint) {
							seekerOrtsteil = ortsteilName
						}
						if planar.MultiPolygonContains(multiPolygon, hiderPoint) {
							hiderOrtsteil = ortsteilName
						}
					}

					var description string

					if hiderOrtsteil == seekerOrtsteil {
						description = "Hider is in " + hiderOrtsteil
						for ortsteilName, multiPolygon := range multiPolygonMap {
							if ortsteilName != hiderOrtsteil {
								fc.Append(geojson.NewFeature(multiPolygon))
							} else {
								log.Debug().Msg("not appending ortsteil " + ortsteilName)
							}
						}
					} else {
						description = "Hider is not in " + hiderOrtsteil
						for ortsteilName, polygon := range multiPolygonMap {
							if ortsteilName == seekerOrtsteil {
								fc.Append(geojson.NewFeature(polygon))
								log.Debug().Msg("appending ortsteil " + ortsteilName)
							}
						}
					}

					log.Debug().Msg("seeker ortsteil is " + seekerOrtsteil + " and hider ortsteil is " + hiderOrtsteil)
					historyItem := models.HistoryInDB{
						LobbyID:     lobby.ID,
						Title:       "Same Ortsteil",
						Description: description,
					}
					result := db.Create(&historyItem)
					if result.Error != nil {
						log.Err(err).Msg("failed creating history item")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.CreateCardDraw(db, 3, 1, lobby.ID, w)
					if err != nil {
						log.Err(result.Error).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.FCToDB(db, lobby, fc)
					if err != nil {
						log.Err(err).Msg("failed to save FC to DB while asking same ortsteil question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				})
				r.Post("/ortsteilLastLetter", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					fc, err := helpers.FCFromDB(lobby)
					if err != nil {
						log.Err(err).Msg("failed to get FC from DB while asking last bezirk letter question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					var zoneCenter orb.Point
					zoneCenter[1] = lobby.ZoneCenterLat
					zoneCenter[0] = lobby.ZoneCenterLon

					var seekerPoint orb.Point
					seekerPoint[1] = lobby.SeekerLat
					seekerPoint[0] = lobby.SeekerLon

					var hiderPoint orb.Point
					hiderPoint[1] = lobby.HiderLat
					hiderPoint[0] = lobby.HiderLon

					multiPolygonMap := make(map[string]orb.MultiPolygon)

					for _, relation := range processedData.Ortsteile {
						ortsteilName := relation.Tags.Find("name")
						if ortsteilName == "" {
							log.Warn().Msg("ortsteil with ID " + fmt.Sprint(relation.ElementID()) + " has empty name field")
							continue
						}
						multiPolygon, err := geo.RelationToMultiPolygon(*relation, processedData.Nodes, processedData.Ways)
						if err != nil {
							log.Err(err).Msg("failed converting relation to polygon")
							continue
						}
						multiPolygonMap[ortsteilName] = multiPolygon
					}

					var seekerOrtsteil string
					var hiderOrtsteil string

					for ortsteilName, multiPolygon := range multiPolygonMap {
						if planar.MultiPolygonContains(multiPolygon, seekerPoint) {
							seekerOrtsteil = ortsteilName
						}
						if planar.MultiPolygonContains(multiPolygon, hiderPoint) {
							hiderOrtsteil = ortsteilName
						}
					}

					var description string

					if hiderOrtsteil[len(hiderOrtsteil)-1] == seekerOrtsteil[len(seekerOrtsteil)-1] {
						description = "Hiders ortsteil ends with " + string(hiderOrtsteil[len(hiderOrtsteil)-1])
						for ortsteilName, polygon := range multiPolygonMap {
							if ortsteilName[len(ortsteilName)-1] != hiderOrtsteil[len(hiderOrtsteil)-1] {
								fc.Append(geojson.NewFeature(polygon))
							}
						}
					} else {
						description = "Hiders ortsteil doesn't end with " + string(seekerOrtsteil[len(seekerOrtsteil)-1])
						for ortsteilName, polygon := range multiPolygonMap {
							if ortsteilName[len(ortsteilName)-1] == seekerOrtsteil[len(seekerOrtsteil)-1] {
								fc.Append(geojson.NewFeature(polygon))
							}
						}
					}

					historyItem := models.HistoryInDB{
						LobbyID:     lobby.ID,
						Title:       "Ortsteil last letter",
						Description: description,
					}

					result := db.Create(&historyItem)
					if result.Error != nil {
						log.Err(err).Msg("failed creating history item")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					log.Debug().Msg("seeker ortsteil is " + seekerOrtsteil + " and hider ortsteil is " + hiderOrtsteil)

					err = helpers.CreateCardDraw(db, 3, 1, lobby.ID, w)
					if err != nil {
						log.Err(result.Error).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.FCToDB(db, lobby, fc)
					if err != nil {
						log.Err(err).Msg("failed to save FC to DB while asking same ortsteil question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				})
				r.Post("/closerToMcDonalds", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					fc, err := helpers.FCFromDB(lobby)

					lobby, isCloser, distance, err := closerOrFurtherFromObject(lobby, fc, processedData, w, processedData.McDonaldsNodes, processedData.McDonaldsWays)
					if err != nil {
						log.Err(err).Msg("failed to get FC from DB while asking last bezirk letter question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}
					var description string
					if isCloser {
						description = "Hider is closer than " + fmt.Sprint(math.Round(distance)) + "m to a McDonald's"
					} else {
						description = "Hider is further than " + fmt.Sprint(math.Round(distance)) + "m from a McDonald's"
					}

					historyItem := models.HistoryInDB{
						LobbyID:     lobby.ID,
						Title:       "Closer to McDonald's",
						Description: description,
					}
					result := db.Create(&historyItem)
					if result.Error != nil {
						log.Err(err).Msg("failed creating history item")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.CreateCardDraw(db, 3, 1, lobby.ID, w)
					if err != nil {
						log.Err(result.Error).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					result = db.Save(&lobby)
					if result.Error != nil {
						log.Err(err).Msg("failed saving lobby")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				})
				r.Post("/closerToIkea", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					fc, err := helpers.FCFromDB(lobby)
					if err != nil {
						log.Err(err).Msg("failed to get FC from DB while asking last bezirk letter question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					lobby, isCloser, distance, err := closerOrFurtherFromObject(lobby, fc, processedData, w, map[osm.NodeID]*osm.Node{}, processedData.IkeaWays)
					var description string
					if isCloser {
						description = "Hider is closer than " + fmt.Sprint(math.Round(distance)) + "m to an IKEA"
					} else {
						description = "Hider is further than " + fmt.Sprint(math.Round(distance)) + "m from an IKEA"
					}

					historyItem := models.HistoryInDB{
						LobbyID:     lobby.ID,
						Title:       "Closer to IKEA",
						Description: description,
					}
					result := db.Create(&historyItem)
					if result.Error != nil {
						log.Err(err).Msg("failed creating history item")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.CreateCardDraw(db, 3, 1, lobby.ID, w)
					if err != nil {
						log.Err(result.Error).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					result = db.Save(&lobby)
					if result.Error != nil {
						log.Err(err).Msg("failed saving lobby")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				})
				r.Post("/closerToSpree", func(w http.ResponseWriter, r *http.Request) {
					lobby, isLobby := r.Context().Value(models.LobbyKey).(models.Lobby)
					if !isLobby {
						fmt.Println(lobby)
						log.Warn().Msg("couldn't cast lobby value from context")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					fc, err := helpers.FCFromDB(lobby)
					if err != nil {
						log.Err(err).Msg("failed to get FC from DB while asking last bezirk letter question")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					lobby, isCloser, distance, err := closerOrFurtherFromOrbLine(lobby, fc, w, processedData.SpreeLineStrings)

					var description string
					if isCloser {
						description = "Hider is closer than " + fmt.Sprint(math.Round(distance)) + "m to the spree"
					} else {
						description = "Hider is further than " + fmt.Sprint(math.Round(distance)) + "m from the spree"
					}

					historyItem := models.HistoryInDB{
						LobbyID:     lobby.ID,
						Title:       "Closer to Spree",
						Description: description,
					}
					result := db.Create(&historyItem)
					if result.Error != nil {
						log.Err(err).Msg("failed creating history item")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					err = helpers.CreateCardDraw(db, 3, 1, lobby.ID, w)
					if err != nil {
						log.Err(result.Error).Msg("failed creating card draw")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					result = db.Save(&lobby)
					if result.Error != nil {
						log.Err(err).Msg("failed saving lobby")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				})
			})
		})
	})
}
