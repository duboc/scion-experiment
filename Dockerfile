FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /eval-server ./cmd/eval-server

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /eval-server /app/eval-server
ENV PORT=8080
ENV GITHUB_REPO=duboc/scion-experiment
EXPOSE 8080
CMD ["/app/eval-server"]
