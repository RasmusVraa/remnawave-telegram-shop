FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS modules
WORKDIR /modules
COPY go.mod go.sum ./
RUN go mod download

FROM --platform=$BUILDPLATFORM node:22-alpine AS cabinet_frontend
WORKDIR /app
COPY web/cabinet/package.json web/cabinet/package-lock.json /app/web/cabinet/
WORKDIR /app/web/cabinet
RUN npm ci
WORKDIR /app
COPY web/cabinet /app/web/cabinet
RUN mkdir -p /app/internal/cabinet/web
WORKDIR /app/web/cabinet
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS builder
WORKDIR /app

COPY --from=modules /go/pkg /go/pkg

COPY . .
COPY --from=cabinet_frontend /app/internal/cabinet/web/dist /app/internal/cabinet/web/dist

RUN apk update && apk add --no-cache ca-certificates tzdata
RUN update-ca-certificates

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=none

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    -o /bin/app ./cmd/app

FROM scratch

ARG VERSION
ARG COMMIT

LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"
LABEL org.opencontainers.image.source="https://github.com/MrMe0ws/remnawave-telegram-shop"
LABEL org.opencontainers.image.description="Remnawave Telegram Shop Bot"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /bin/app /app/app

COPY --from=builder /app/db /db
COPY --from=builder /app/translations /translations
COPY --from=builder /app/web/cabinet/src/i18n /translations/cabinet/i18n

USER 1000

ENV DISABLE_ENV_FILE=true

CMD ["/app/app"]