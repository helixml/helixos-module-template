# One image, one process: the Go binary serves the API, MCP, events and the
# static UI, with PocketBase data under /data (the host's durable volume).

FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG GIT_SHA=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/helixml/helixos-module-template/internal/module.GitSHA=${GIT_SHA}" \
      -o /module .

FROM gcr.io/distroless/static-debian12
COPY --from=build /module /module
COPY --from=web /web/out /web
ENV MODULE_WEB_DIR=/web
EXPOSE 8080
ENTRYPOINT ["/module", "serve", "--http", "0.0.0.0:8080", "--dir", "/data/pb_data"]
