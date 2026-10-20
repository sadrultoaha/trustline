# ---- web build -------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- go build --------------------------------------------------------------
FROM golang:1.26-alpine AS go
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/trustline ./cmd/trustline

# ---- runtime ---------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget && adduser -D -u 10001 app
WORKDIR /app
COPY --from=go /out/trustline /app/trustline
COPY --from=web /web/dist /app/web
USER app
EXPOSE 8080
ENTRYPOINT ["/app/trustline"]
CMD ["serve"]
