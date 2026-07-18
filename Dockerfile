FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
ARG VITE_SOURCE_CODE_URL
ENV VITE_SOURCE_CODE_URL=$VITE_SOURCE_CODE_URL
RUN npm run build

FROM golang:1.26.5-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/image-agent ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/image-agent-admin ./cmd/admin \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/image-agent-secrets ./cmd/secrets

FROM alpine:3.22
RUN addgroup -S imageagent && adduser -S -G imageagent imageagent
WORKDIR /app
COPY --from=go-build /out/image-agent /usr/local/bin/image-agent
COPY --from=go-build /out/image-agent-admin /usr/local/bin/image-agent-admin
COPY --from=go-build /out/image-agent-secrets /usr/local/bin/image-agent-secrets
COPY --from=web-build /src/web/dist ./web/dist
RUN mkdir -p /app/data/assets && chown -R imageagent:imageagent /app
USER imageagent
EXPOSE 8888
ENTRYPOINT ["image-agent"]
