FROM --platform=amd64 golang:1.23 AS builder

# Install CA certificates
RUN apt-get update && apt-get install -y ca-certificates
RUN apt-get install -y build-essential


# Move to working directory (/build).
RUN mkdir /build
WORKDIR /build

# Copy and download dependency using go mod.
COPY go.mod go.sum ./
RUN go mod download

# Copy the code into the container.
COPY . .

# Set necessary environment variables needed for our image and build the API server.
ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
# RUN go build -ldflags="-s -w" -o onepixel ./src/main.go
RUN make build DOCS=false

FROM --platform=amd64 debian:bookworm-slim

LABEL maintainer="Arnav Gupta <championswimmer@gmail.com> (https://arnav.tech)"
LABEL description="Midpoint Place API to create groups and find a midpoint to meet for social meetups"

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

RUN mkdir /app

# Copy binary and config files from /build to root folder of scratch container.
COPY --from=builder ["/build/.env.*", "/app/"]
COPY --from=builder ["/build/bin/midpoint_place", "/app/"]

# Command to run when starting the container.
WORKDIR /app 
CMD [ "./midpoint_place" ]