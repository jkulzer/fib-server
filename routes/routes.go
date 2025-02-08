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

	"github.com/jkulzer/osm"
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
					boundaryFromLS, err := geo.RingFromLineStrings(boundaryLineStrings)
					if err != nil {
						log.Err(err).Msg("")
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
					}
					berlinBoundary := orb.Polygon(boundaryFromLS)
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
						w.WriteHeader(http.StatusInternalServerError)
						w.Write(nil)
						return
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
			r.Get("/closeRoutes", func(w http.ResponseWriter, r *http.Request) {
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
						Name:    route.Tags.Find("name"),
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

			memberIteration:
				for _, member := range route.Members {
					if member.Type == "node" {
						memberNodeID, err := member.ElementID().NodeID()
						if err != nil {
							log.Err(err).Msg("element id: " + fmt.Sprint(member.ElementID()))
							continue memberIteration
						}
						memberNode := processedData.Nodes[memberNodeID]
						if memberNode.Tags.Find("railway") != "stop" {
							continue memberIteration
						}
						stopPositionPoint := helpers.NodeToPoint(*memberNode)
						if orbGeo.DistanceHaversine(stopPositionPoint, zoneCenter) <= sharedModels.HidingZoneRadius {
							log.Debug().Msg("hider could be at stop " + memberNode.Tags.Find("name") + " with ID " + fmt.Sprint(memberNode.ElementID()))
							circle := helpers.NewCircle(stopPositionPoint, sharedModels.HidingZoneRadius*2)
							fc.Append(geojson.NewFeature(circle))
						} else {
							log.Debug().Msg("hider is not at stop " + memberNode.Tags.Find("name") + " with ID " + fmt.Sprint(memberNode.ElementID()))
							inverseCircle := helpers.NewInverseCircle(stopPositionPoint, sharedModels.HidingZoneRadius*2)
							fc.Append(geojson.NewFeature(inverseCircle))
						}
					}
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
					inverseCircleFeature := geojson.NewFeature(inverseCircle)
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
					circleFeature := geojson.NewFeature(circle)
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
			r.Post("/thermometer/start", func(w http.ResponseWriter, r *http.Request) {
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

				result = db.Save(&lobby)
				if result.Error != nil {
					log.Err(result.Error).Msg("")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(nil)
			})
			r.Post("/thermometer/end", func(w http.ResponseWriter, r *http.Request) {
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

				if orbGeo.DistanceHaversine(seekerPoint, hiderPoint) < orbGeo.DistanceHaversine(thermometerStartPoint, hiderPoint) {
					if thermometerBearing > 0 {
						thermometerBearing = thermometerBearing - 180
					} else {
						thermometerBearing = thermometerBearing + 180
					}
				}

				boxFrontLeft := orbGeo.PointAtBearingAndDistance(orbGeo.PointAtBearingAndDistance(seekerPoint, thermometerBearing, 30000), leftBearing, 30000)
				boxFrontRight := orbGeo.PointAtBearingAndDistance(orbGeo.PointAtBearingAndDistance(seekerPoint, thermometerBearing, 30000), rightBearing, 30000)
				boxLeft := orbGeo.PointAtBearingAndDistance(seekerPoint, leftBearing, 30000)
				boxRight := orbGeo.PointAtBearingAndDistance(seekerPoint, rightBearing, 30000)

				boxPolygon := orb.Polygon{orb.Ring{boxFrontLeft, boxFrontRight, boxRight, boxLeft, boxFrontLeft}}

				fc, err := helpers.FCFromDB(lobby)
				if err != nil {
					log.Err(result.Error).Msg("")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}
				fc.Append(geojson.NewFeature(boxPolygon))
				lobby.ThermometerDistance = 0

				err = helpers.FCToDB(db, lobby, fc)
				if err != nil {
					log.Err(result.Error).Msg("")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(nil)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write(nil)
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
					bezirkRing, err := geo.RingFromLineStrings(bezirkLineStrings)
					if err == nil {
						bezirkPolygon := orb.Polygon(bezirkRing)
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
			r.Post("/trainService", func(w http.ResponseWriter, r *http.Request) {
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

				isOnRoute := false
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
							continue memberIteration
						}
						if memberNode.Tags.Find("railway") != "stop" {
							continue memberIteration
						}
						stopPositionPoint := helpers.NodeToPoint(*memberNode)
						if orbGeo.DistanceHaversine(stopPositionPoint, zoneCenter) <= sharedModels.HidingZoneRadius {
							log.Debug().Msg("hider is at stop" + memberNode.Tags.Find("name") + " with ID " + fmt.Sprint(memberNode.ElementID()))
							isOnRoute = true
						} else {
							log.Debug().Msg("hider is not at stop" + memberNode.Tags.Find("name") + " with ID " + fmt.Sprint(memberNode.ElementID()))
						}
					}
				}

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
						if isOnRoute {
							inverseCircle := helpers.NewInverseCircle(stopPositionPoint, sharedModels.HidingZoneRadius)
							fc.Append(geojson.NewFeature(inverseCircle))
						} else {
							circle := helpers.NewCircle(stopPositionPoint, sharedModels.HidingZoneRadius)
							fc.Append(geojson.NewFeature(circle))
						}
					}
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
			r.Post("/sameOrtsteil", func(w http.ResponseWriter, r *http.Request) {
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
				var seekerOrtsteilName string

				ortsteilPolygons := make(map[string]orb.Polygon)
				for _, ortsteilRelation := range processedData.Ortsteile {
					ortsteilName := ortsteilRelation.Tags.Find("name")
					var ortsteilLineStrings []orb.LineString
					for _, member := range ortsteilRelation.Members {
						if member.Type == "way" {
							wayID, err := member.ElementID().WayID()
							if err != nil {
								log.Err(err).Msg("")
								continue
							}
							way := processedData.Ways[wayID]
							lineString := geo.LineStringFromWay(way, processedData.Nodes)
							ortsteilLineStrings = append(ortsteilLineStrings, lineString)
						}
					}

					ortsteilRing, err := geo.RingFromLineStrings(ortsteilLineStrings)
					if err == nil {
						ortsteilPolygon := orb.Polygon(ortsteilRing)
						ortsteilPolygons[ortsteilName] = ortsteilPolygon
						seekerInside := planar.PolygonContains(ortsteilPolygon, seekerPoint)
						hiderInside := planar.PolygonContains(ortsteilPolygon, hiderPoint)

						if seekerInside && hiderInside {
							sameBezirk = true
							seekerOrtsteilName = ortsteilName
						} else if seekerInside && !hiderInside {
							sameBezirk = false
							seekerOrtsteilName = ortsteilName
						} else if !seekerInside && hiderInside {
							sameBezirk = false
						}
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
					log.Info().Msg("it's a hit!")
					for ortsteilName, ortsteilPolygon := range ortsteilPolygons {
						if ortsteilName != seekerOrtsteilName {
							fc.Append(geojson.NewFeature(ortsteilPolygon))
							err = helpers.FCToDB(db, lobby, fc)
						}
					}
					// different bezirk, only color bezirk of seeker
				} else {
					log.Info().Msg("it's a hit!")
					seekerOrtsteil := ortsteilPolygons[seekerOrtsteilName]
					fc.Append(geojson.NewFeature(seekerOrtsteil))
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
