# syntax=docker/dockerfile:1

# builder: dist を生成する
FROM node:26 AS builder
WORKDIR /src

# 企業ネットワークの TLS インスペクション CA など、npm ci が参照する追加 CA 証明書を任意で
# 注入する。未指定時は何もしない (compose.yaml の secrets.custom_ca 経由、環境変数
# CUSTOM_CA_CERT にホスト側の CA バンドルパスを指定する)。
RUN --mount=type=secret,id=custom_ca,required=false \
    if [ -s /run/secrets/custom_ca ]; then \
      cp /run/secrets/custom_ca /usr/local/share/ca-certificates/custom-ca.crt && \
      update-ca-certificates && \
      npm config set cafile /etc/ssl/certs/ca-certificates.crt; \
    fi

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# VITE_API_BASE はビルド時に静的置換されるため build-arg で受け取る。
ARG VITE_API_BASE=http://localhost:8089
ENV VITE_API_BASE=${VITE_API_BASE}
RUN npm run build

# runtime: nginx で dist を静的配信する
FROM nginx:alpine
COPY --from=builder /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8088
