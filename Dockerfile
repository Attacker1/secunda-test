# --- build stage ---
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/api /app/api
COPY config.yaml /app/config.yaml
EXPOSE 8080
ENTRYPOINT ["/app/api", "-config", "/app/config.yaml"]
