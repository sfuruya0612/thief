# syntax=docker/dockerfile:1

# builder: 静的バイナリをビルドする (CGO_ENABLED=0)
FROM golang:1.25 AS builder
WORKDIR /src

# 企業ネットワークの TLS インスペクション CA など、go mod download が参照する追加 CA 証明書を
# 任意で注入する。未指定時は何もしない (compose.yaml の secrets.custom_ca 経由、
# 環境変数 CUSTOM_CA_CERT にホスト側の CA バンドルパスを指定する)。
RUN --mount=type=secret,id=custom_ca,required=false \
    if [ -s /run/secrets/custom_ca ]; then \
      cp /run/secrets/custom_ca /usr/local/share/ca-certificates/custom-ca.crt && \
      update-ca-certificates; \
    fi

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

# runtime: distroless static (root バリアント、HOME=/root)。
# CA 証明書は同梱済みのため AWS/SSO への TLS 通信に追加設定は不要。
FROM gcr.io/distroless/static-debian12:latest
COPY --from=builder /out/server /server

EXPOSE 8089
ENTRYPOINT ["/server"]
