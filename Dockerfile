# ---- build ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime ----
FROM alpine:3.22
RUN apk add --no-cache ca-certificates ffmpeg \
    && addgroup -S -g 10001 music \
    && adduser -S -D -H -u 10001 -G music music \
    && mkdir -p /data/storage \
    && chown -R music:music /data
COPY --from=build /out/server /server
# The trimmed binary cannot find source-relative config files; use the safe templates.
COPY --from=build /src/config/example /config/resources
ENV CONFIG_PATH=/config/resources
USER music:music
EXPOSE 8080
ENTRYPOINT ["/server"]
CMD ["serve"]
