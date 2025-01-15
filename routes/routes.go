package routes

import (
	"gorm.io/gorm"

	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jkulzer/fib-server/controllers"
	"github.com/jkulzer/fib-server/helpers"
	"github.com/jkulzer/fib-server/models"
	"github.com/jkulzer/fib-server/sharedModels"

	chi "github.com/go-chi/chi/v5"

	"github.com/rs/zerolog/log"
)

func Router(r chi.Router, db *gorm.DB) {
	r.Route("/register", func(r chi.Router) {
		r.Post("/",
			func(w http.ResponseWriter, r *http.Request) {
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
						fmt.Println("Failed to hash password")
					}

					userName := models.UserAccount{
						Name:     loginInfo.Username,
						Password: hashedPassword,
					}
					// tries to create the user in the db
					result := db.Create(&userName)

					// if the user creation fails,
					if result.Error != nil {
						fmt.Println("Duplicate Username")
						w.WriteHeader(http.StatusBadRequest)
						w.Write(nil)
					} else {
						w.WriteHeader(http.StatusCreated)
						w.Write(nil)
					}
				}
			},
		)
	})
	r.Route("/login", func(r chi.Router) {
		r.Post("/",
			func(w http.ResponseWriter, r *http.Request) {
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
			},
		)
	})
	r.Route("/lobby", func(r chi.Router) {
		r.Post("/create",
			func(w http.ResponseWriter, r *http.Request) {
				body, err := helpers.ReadHttpResponse(r.Body)
				if err != nil {
					log.Warn().Msg("failed to read http response")
				}

				var lobbyCreationInfo sharedModels.CreateLobby
				err = json.Unmarshal(body, &lobbyCreationInfo)
				if err != nil {
					log.Warn().Msg("failed to parse json of lobby creating request")
				} else {
					creatorSession := &models.Session{Token: lobbyCreationInfo.Token}
					result := db.First(&creatorSession)

					// if the user creation fails,
					if result.Error != nil {
						log.Info().Msg("failed to find token, unauthenticated")
						w.WriteHeader(http.StatusBadRequest)
						w.Write(nil)
					} else {

						charsetString := "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"

						lobbyToken := helpers.RandomString(6, charsetString)

						var lobby models.Lobby

						// deletes all other lobbies owned by the creator of the current lobby
						// this ensures that no zombie lobbies exist in the database
						db.Where("creator_id = ?", creatorSession.UserAccount.ID).Delete(&models.Lobby{})
						// := models.Lobby{
						// 	Creator: creatorSession.UserAccount,
						// }
						lobby.Token = lobbyToken

						result := db.Create(&lobby)
						if result.Error != nil {
							log.Err(err).Msg("failed to create lobby in database")
						}

						lobbyCreationResponse := sharedModels.LobbyCreationResponse{
							LobbyToken: lobbyToken,
						}

						fmt.Println("Created lobby with token " + lobbyToken)
						marshalledJson, err := json.Marshal(lobbyCreationResponse)
						if err != nil {
							log.Err(err).Msg("failed to marshal lobby creation response")
						}
						w.WriteHeader(http.StatusCreated)
						w.Write(marshalledJson)
					}
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
						w.WriteHeader(http.StatusAccepted)
						w.Write(nil)
					}
				}
			},
		)
	})
	r.Route("/location", func(r chi.Router) {
		r.Post("/create",
			func(w http.ResponseWriter, r *http.Request) {
				body, err := helpers.ReadHttpResponse(r.Body)
				if err != nil {
					log.Warn().Msg("failed to read http response")
				}

				var lobbyCreationInfo sharedModels.CreateLobby
				err = json.Unmarshal(body, &lobbyCreationInfo)
				if err != nil {
					log.Warn().Msg("failed to parse json of lobby creating request")
				} else {
					creatorSession := &models.Session{Token: lobbyCreationInfo.Token}
					result := db.First(&creatorSession)

					// if the user creation fails,
					if result.Error != nil {
						log.Info().Msg("failed to find token, unauthenticated")
						w.WriteHeader(http.StatusBadRequest)
						w.Write(nil)
					} else {

						charsetString := "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"

						lobbyToken := helpers.RandomString(6, charsetString)

						var lobby models.Lobby

						// deletes all other lobbies owned by the creator of the current lobby
						// this ensures that no zombie lobbies exist in the database
						db.Where("creator_id = ?", creatorSession.UserAccount.ID).Delete(&models.Lobby{})
						// := models.Lobby{
						// 	Creator: creatorSession.UserAccount,
						// }
						lobby.Token = lobbyToken

						result := db.Create(&lobby)
						if result.Error != nil {
							log.Err(err).Msg("failed to create lobby in database")
						}

						lobbyCreationResponse := sharedModels.LobbyCreationResponse{
							LobbyToken: lobbyToken,
						}

						fmt.Println("Created lobby with token " + lobbyToken)
						marshalledJson, err := json.Marshal(lobbyCreationResponse)
						if err != nil {
							log.Err(err).Msg("failed to marshal lobby creation response")
						}
						w.WriteHeader(http.StatusCreated)
						w.Write(marshalledJson)
					}
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
						w.WriteHeader(http.StatusAccepted)
						w.Write(nil)
					}
				}
			},
		)
	})
}
