FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /dredd ./cmd/dredd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /dredd /usr/local/bin/dredd
COPY migrations/ /migrations/
EXPOSE 8750
ENTRYPOINT ["dredd"]
