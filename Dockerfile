# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go backend with embedded frontend
FROM golang:1.22-alpine AS backend
RUN apk add --no-cache git
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend/ ./backend/
COPY --from=frontend /src/frontend/dist ./backend/internal/frontend/static/
ARG VERSION=dev
RUN cd backend && CGO_ENABLED=0 go build -tags embed -ldflags "-s -w -X main.version=${VERSION}" -o /agent-racer-server ./cmd/server

# Stage 3: Minimal runtime image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S app -G app
USER app
COPY --from=backend /agent-racer-server /usr/local/bin/agent-racer-server
EXPOSE 8080
ENTRYPOINT ["agent-racer-server"]
