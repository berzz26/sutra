FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/sutra ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/sutra /app/sutra
EXPOSE 8080
ENTRYPOINT ["/app/sutra"]
