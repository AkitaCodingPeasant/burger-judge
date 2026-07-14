# 第一階段只負責編譯靜態 Go API，避免把編譯工具帶入執行映像。
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/regs ./cmd/api

# DinD 讓 API 容器內的判題器可沿用既有 Docker CLI 與 bind mount 邏輯。
FROM docker:27-dind

WORKDIR /app
COPY --from=builder /out/regs /app/regs
COPY api /app/api
COPY migrations /app/migrations
COPY test-framework /app/test-framework
COPY deploy/docker-entrypoint.sh /usr/local/bin/regs-entrypoint
# The repository may be checked out on Windows with CRLF line endings.  A CR
# after the shebang makes Linux look for `/bin/sh\r` and the container fails
# with the misleading "no such file or directory" error.
RUN sed -i 's/\r$//' /usr/local/bin/regs-entrypoint \
    && chmod +x /usr/local/bin/regs-entrypoint

EXPOSE 8080
ENTRYPOINT ["regs-entrypoint"]

