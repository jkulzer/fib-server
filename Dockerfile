FROM golang:1.23

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY . .

RUN go mod download

RUN go build -o /fib-server

RUN wget https://download.geofabrik.de/europe/germany/berlin-latest.osm.pbf

EXPOSE 3001

# Run
CMD ["/fib-server"]
