# CampusClaw api（Go）镜像
FROM golang:1.23-alpine AS build
WORKDIR /src
# 国内构建环境可换 GOPROXY；默认走官方代理
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/campusclaw-api ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /out/campusclaw-api /app/campusclaw-api
RUN mkdir -p /app/uploads && chown appuser:appuser /app/uploads
USER appuser
EXPOSE 8081
CMD ["/app/campusclaw-api"]
