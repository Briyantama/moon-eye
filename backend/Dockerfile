FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TODO: build separate binaries per service when implementations are complete.
RUN go build -o /out/finance-api ./cmd/api/finance-api \
 && go build -o /out/auth-service ./cmd/api/auth-service \
 && go build -o /out/sync-service ./cmd/api/sync-service \
 && go build -o /out/worker-service ./cmd/api/worker-service

FROM alpine:3.19 AS runtime

WORKDIR /app

COPY --from=builder /out/finance-api /usr/local/bin/finance-api
COPY --from=builder /out/auth-service /usr/local/bin/auth-service
COPY --from=builder /out/sync-service /usr/local/bin/sync-service
COPY --from=builder /out/worker-service /usr/local/bin/worker-service
COPY configs ./configs

# Select service via SERVICE_NAME environment variable.
ENV SERVICE_NAME=finance-api

CMD ["/bin/sh", "-c", "exec /usr/local/bin/${SERVICE_NAME}"]

