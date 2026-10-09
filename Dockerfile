# syntax=docker/dockerfile:1
# Incident Status Dashboard: one container serving the API and the static UI.
# Build context: /workspace (needs both backend/ and frontend/).

FROM golang:1.26 AS build
WORKDIR /src
COPY backend/go.mod ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/incidentdash .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/incidentdash /app/incidentdash
COPY frontend/ /app/frontend/
USER nonroot:nonroot
# Cloud Run injects PORT (default 8080); the server honours it when -addr is not set.
EXPOSE 8080
ENTRYPOINT ["/app/incidentdash", "-static", "/app/frontend"]
