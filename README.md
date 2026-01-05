# thermalprinter_go

## 開発用ホットリロード (air)
- インストール: `go install github.com/air-verse/air@latest`
- 起動: `air`
- 設定: `.air.toml` を参照（`go run main.go` を実行し `.env` も読み込み可能）

## HTTPS/HTTP 切り替え
- 環境変数 (`.env`): `ECHO_SCHEME` (`http`/`https`), `ECHO_PORT`, `ECHO_CERT_FILE`, `ECHO_KEY_FILE`
- 既定値: `ECHO_SCHEME=http`, ポート `1323`, 証明書 `server.crt`, 鍵 `server.key`
- 自己署名証明書の生成: `make cert`（`server.crt` / `server.key` を作成）

## systemd サービス登録
- `make install-service` でバイナリをビルドし、`thermalprinter-go.service` を `/etc/systemd/system/` へ登録して再起動します。
- 環境変数を上書きする場合は `/etc/default/thermalprinter-go` を用意してください。
