# syntax=docker/dockerfile:1

# builder: 静的バイナリをビルドする (CGO_ENABLED=0)
# distroless/base-debian12 (bookworm) と glibc ABI を合わせるため bookworm タグを固定する。
FROM golang:1.25-bookworm AS builder
ARG TARGETARCH
WORKDIR /src

# 企業ネットワークの TLS インスペクション CA など、go mod download / aws CLI インストーラの
# ダウンロードが参照する追加 CA 証明書を任意で注入する。未指定時は何もしない (compose.yaml の
# secrets.custom_ca 経由、環境変数 CUSTOM_CA_CERT にホスト側の CA バンドルパスを指定する)。
RUN --mount=type=secret,id=custom_ca,required=false \
    if [ -s /run/secrets/custom_ca ]; then \
      cp /run/secrets/custom_ca /usr/local/share/ca-certificates/custom-ca.crt && \
      update-ca-certificates; \
    fi

# aws CLI v2 をインストールする。handleSSOLogin (internal/api/handlers_sso.go) が
# 子プロセスとして `aws sso login` を起動するため、実行イメージにも aws CLI 本体が必要。
RUN apt-get update && apt-get install -y --no-install-recommends unzip curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
         amd64) awsArch=x86_64; libDir=x86_64-linux-gnu ;; \
         arm64) awsArch=aarch64; libDir=aarch64-linux-gnu ;; \
         *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
       esac \
    && curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${awsArch}.zip" -o /tmp/awscliv2.zip \
    && unzip -q /tmp/awscliv2.zip -d /tmp \
    && /tmp/aws/install -i /usr/local/aws-cli -b /usr/local/bin \
    && rm -rf /tmp/awscliv2.zip /tmp/aws \
    && mkdir -p /out/lib \
    && cp "/lib/${libDir}/libz.so.1" /out/lib/libz.so.1

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/thief

# runtime: distroless base (glibc 同梱、root バリアント、HOME=/root)。
# CA 証明書は同梱済みのため AWS/SSO への TLS 通信に追加設定は不要。
# aws CLI 本体は glibc 依存。libz.so.1 のみ base イメージに不足しているため /usr/local/lib に
# 追加コピーし、triplet ディレクトリ名 (aarch64/x86_64-linux-gnu) を気にせず解決できるよう
# LD_LIBRARY_PATH で参照させる。/usr/local/aws-cli はディレクトリ単位でコピーし、内部の
# シンボリックリンク構造 (current -> v2/<version>、bin/aws -> ../dist/aws) を保持する。
# aws 本体 (PyInstaller バンドル) は実行ファイルと同じディレクトリの libpython*.so を
# 相対参照で探すため、シンボリックリンクを解体するとロードに失敗する。
FROM gcr.io/distroless/base-debian12:latest
COPY --from=builder /usr/local/aws-cli /usr/local/aws-cli
COPY --from=builder /out/lib/libz.so.1 /usr/local/lib/libz.so.1
COPY --from=builder /out/server /server

ENV PATH="/usr/local/aws-cli/v2/current/bin:${PATH}" \
    LD_LIBRARY_PATH="/usr/local/lib"
EXPOSE 8089
ENTRYPOINT ["/server", "server"]
