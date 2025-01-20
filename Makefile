run: format execute

generate:
	go run github.com/swaggo/swag/cmd/swag@latest init
	swagger generate client -f docs/swagger.yaml --model-package=clientModel
	swagger generate client -f docs/swagger.yaml --model-package=clientModel -A my-api


format:
	go fmt .

execute: 
	go run .
