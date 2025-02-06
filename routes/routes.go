package routes

import (
	"gorm.io/gorm"

	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/jkulzer/fib-server/controllers"
	"github.com/jkulzer/fib-server/geo"
	"github.com/jkulzer/fib-server/helpers"
	"github.com/jkulzer/fib-server/models"
	"github.com/jkulzer/fib-server/sharedModels"

	"github.com/paulmach/orb"
	orbGeo "github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"github.com/paulmach/orb/simplify"

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
		r.Post("/create",
			func(w http.ResponseWriter, r *http.Request) {
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
					berlinBoundary := orb.Polygon(geo.RingFromLineStrings(boundaryLineStrings))
					berlinBoundary[0] = append(berlinBoundary[0], sharedModels.LeftTopPoint())
					berlinBoundary[0].Reverse()
					berlinBoundary[0] = append(berlinBoundary[0],
						sharedModels.LeftTopPoint(),
						sharedModels.LeftBottomPoint(),
						sharedModels.RightBottomPoint(),
						sharedModels.RightTopPoint(),
						sharedModels.LeftTopPoint(),
					)
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
					}

					lobbyCreationResponse := sharedModels.LobbyCreationResponse{
						LobbyToken: lobbyToken,
					}

					fmt.Println(lobbyCreationResponse)

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
		r.Post("/join",
			func(w http.ResponseWriter, r *http.Request) {
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
		r.Route("/{index}/questions", func(r chi.Router) {
			r.Use(AuthMiddleware(db))
			r.Post("/radar/{radius}", func(w http.ResponseWriter, r *http.Request) {
				lobbyToken := chi.URLParam(r, "index")
				// regex for verifying the lobby token
				lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
				// if the input is valid
				if !lobbyTokenRegex.MatchString(lobbyToken) {
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
				}
				// finds lobby in DB
				var lobby models.Lobby
				result := db.Where("token = ?", lobbyToken).First(&lobby)
				// if lobby can't be found
				if result.Error != nil {
					w.WriteHeader(http.StatusBadRequest)
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

				if distanceSeekerHider < radius {
					// it's a hit!
					inverseCircle := helpers.NewInverseCircle(seekerPoint, radius)
					fc, err := helpers.FCFromDB(lobby)
					if err != nil {
						log.Err(err).Msg("")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}
					inverseCircleFeature := geojson.NewFeature(inverseCircle.Geometry)
					fc.Append(inverseCircleFeature)
					err = helpers.FCToDB(db, lobby, fc)
					if err != nil {
						log.Err(err).Msg("")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}
					w.WriteHeader(http.StatusOK)
					w.Write(nil)
				} else {
					// it's a miss!
					circle := helpers.NewCircle(seekerPoint, radius)
					circleFeature := geojson.NewFeature(circle.Geometry)
					fc, err := helpers.FCFromDB(lobby)
					fc.Append(circleFeature)
					if err != nil {
						log.Err(err).Msg("")
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
				}
			})
			r.Post("/sameBezirk", func(w http.ResponseWriter, r *http.Request) {
				lobbyToken := chi.URLParam(r, "index")
				// regex for verifying the lobby token
				lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
				// if the input is valid
				if !lobbyTokenRegex.MatchString(lobbyToken) {
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
					return
				}
				// finds lobby in DB
				var lobby models.Lobby
				result := db.Where("token = ?", lobbyToken).First(&lobby)
				// if lobby can't be found
				if result.Error != nil {
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

				var sameBezirk bool
				var seekerBezirkName string

				bezirkPolygons := make(map[string]orb.Polygon)
				for _, bezirkRelation := range processedData.Bezirke {
					bezirkName := bezirkRelation.Tags.Find("name")
					var bezirkLineStrings []orb.LineString
					for _, member := range bezirkRelation.Members {
						if member.Type == "way" {
							wayID, err := member.ElementID().WayID()
							if err != nil {
								log.Err(err).Msg("")
								continue
							}
							way := processedData.Ways[wayID]
							lineString := geo.LineStringFromWay(way, processedData.Nodes)
							bezirkLineStrings = append(bezirkLineStrings, lineString)
						}
					}
					bezirkPolygon := orb.Polygon(geo.RingFromLineStrings(bezirkLineStrings))
					bezirkPolygons[bezirkName] = bezirkPolygon
					seekerInside := planar.PolygonContains(bezirkPolygon, seekerPoint)
					hiderInside := planar.PolygonContains(bezirkPolygon, hiderPoint)
					if seekerInside && hiderInside {
						sameBezirk = true
						seekerBezirkName = bezirkName
					} else if seekerInside && !hiderInside {
						sameBezirk = false
						seekerBezirkName = bezirkName
					} else if !seekerInside && hiderInside {
						sameBezirk = false
					}
				}
				fc, err := helpers.FCFromDB(lobby)
				if err != nil {
					log.Err(err).Msg("")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				// same bezirk, color all other bezirke
				if sameBezirk {
					for bezirkName, bezirkPolygon := range bezirkPolygons {
						if bezirkName != seekerBezirkName {
							fc.Append(geojson.NewFeature(bezirkPolygon))
							err = helpers.FCToDB(db, lobby, fc)
						}
					}
					// different bezirk, only color bezirk of seeker
				} else {
					seekerBezirk := bezirkPolygons[seekerBezirkName]
					fc.Append(geojson.NewFeature(seekerBezirk))
					err = helpers.FCToDB(db, lobby, fc)
				}
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
		r.Get("/{index}/map", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby)
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(lobby.ExcludedArea))
		})
		r.Get("/{index}/phase", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby)
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
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
		r.Get("/{index}/readiness", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if lobbyTokenRegex.MatchString(lobbyToken) {
				// finds lobby in DB
				var lobby models.Lobby
				result := db.Where("token = ?", lobbyToken).First(&lobby)
				// if lobby can't be found
				if result.Error != nil {
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
				} else {
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
				}
				// if the input is not valid
			} else {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
			}
		})
		r.Put("/{index}/saveLocation", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby)
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
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

			result = db.Save(&lobby)
			if result.Error != nil {
				log.Err(err).Msg("failed to save location to DB with error " + fmt.Sprint(result.Error))
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(nil)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write(nil)
		})
		r.Put("/{index}/saveHidingZone", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby)
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
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

			result = db.Save(&lobby)
			if result.Error != nil {
				log.Err(err).Msg("failed to save readiness info to DB  with error " + fmt.Sprint(result.Error))
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(nil)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write(nil)
		})
		r.Put("/{index}/readiness", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby)
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
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
				log.Warn().Msg("user made reqest to set readiness for lobby " + fmt.Sprint(lobbyToken) + " and isn't hider or seeker")
				w.WriteHeader(http.StatusBadRequest)
			}

			result = db.Save(&lobby)
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

					<-runTimer.C
					fmt.Println("Hiding Time Finished")
					var lobby models.Lobby
					result := db.Where("token = ?", lobbyToken).First(&lobby)
					if result.Error != nil {
						log.Err(err).Msg("failed to save finished hiding time to DB")
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
		r.Get("/{index}/runStartTime", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				// if the input is not valid
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby)
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
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
		r.Get("/{index}/roles", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if lobbyTokenRegex.MatchString(lobbyToken) {
				// finds lobby in DB
				var lobby models.Lobby
				result := db.Where("token = ?", lobbyToken).First(&lobby)
				// if lobby can't be found
				if result.Error != nil {
					w.WriteHeader(http.StatusBadRequest)
					w.Write(nil)
				} else {
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
				}
				// if the input is not valid
			} else {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
			}
		})
		r.Post("/{index}/selectRole", func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			userID, isUint := r.Context().Value(models.UserIDKey).(uint)
			if isUint == false {
				log.Warn().Msg("failed to convert userID to uint in role selection")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(nil)
				return
			}
			// if the input lobby token is not valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby)
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
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
			result = db.Save(&lobby)
			if result.Error != nil {
				log.Warn().Msg("failed to save role in db")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(nil)
				return
			}
		})
	})
}
