# CampusClaw api（Go）镜像
FROM golang:1.23-alpine AS build
WORKDIR /src
# 构建环境默认走国内模块代理（可用 --build-arg 覆盖）
ARG GOPROXY=https://goproxy.cn,direct
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
