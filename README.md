# thermalprinter-go

USB 接続した ESC/POS 対応サーマルプリンターを、HTTP API 経由で操作するための Go 製サーバーです。
JSON で印刷レイアウトを送ると、テキスト、罫線、QR コード、紙送り、カットを指定順に出力します。

主に Linux 上で `/dev/usb/lp0` として認識されるプリンターを想定しています。既定の文字コードは Shift-JIS で、日本語印刷にも対応します。

## 必要なもの

- Go 1.24 以降
- ESC/POS 対応サーマルプリンター
- Linux の USB プリンターデバイス（既定: `/dev/usb/lp0`）
- プリンターに対応するドライバー

## セットアップ

プリンターを接続し、デバイスを確認します。

```bash
make list-devices
```

通常ユーザーからプリンターを使用する場合は、そのユーザーを `lp` グループへ追加し、再ログインまたは再起動してください。

```bash
make add-lp-group
```

設定ファイルを作成し、接続環境に合わせて編集します。

```bash
cp .env.example .env
```

開発時は次のコマンドで起動できます。

```bash
make run
```

サーバーは既定で `http://localhost:1323` をリッスンします。

## 印刷 API

`POST /print` に `layout` 配列を含む JSON を送信します。各要素は配列の先頭から順に印刷されます。

```bash
curl -X POST http://localhost:1323/print \
  -H 'Content-Type: application/json' \
  -d '{
    "layout": [
      {
        "type": "text",
        "text": "ご利用ありがとうございます",
        "align": "center",
        "bold": true,
        "width": 2,
        "height": 2
      },
      { "type": "line" },
      { "type": "text", "text": "受付番号: 001" },
      { "type": "qr", "qr": "https://example.com/ticket/001" },
      { "type": "feed", "feed": 2 },
      { "type": "cut" }
    ]
  }'
```

成功時は `{"status":"printed"}` を返します。JSON が不正、または `layout` が空の場合は `400 Bad Request`、プリンターデバイスや印刷処理でエラーが発生した場合は `500 Internal Server Error` を返します。

### レイアウト要素

| `type` | 用途 | 主なフィールド |
| --- | --- | --- |
| `text` | テキストを印刷 | `text`, `align`, `bold`, `underline`, `invert`, `width`, `height` |
| `line` | 区切り線を印刷 | なし |
| `qr` | QR コードを中央揃えで印刷 | `qr`（未指定時は `url`、`text` の順に使用） |
| `feed` | 紙送り | `feed`（省略または 0 以下の場合は 1） |
| `space` | 1 行改行 | なし |
| `cut` | 用紙をカット | なし |

`text` の指定値は次のとおりです。

- `align`: `left`（既定）、`center`、`right`
- `bold`: 太字
- `underline`: 下線
- `invert`: 白黒反転
- `width`, `height`: 文字の横・縦倍率。省略時は `1`
- `text` に改行を含めた場合は、行ごとに印刷されます

より詳しい定義は [openapi.yaml](./openapi.yaml)、追加の送信例は [rest.rest](./rest.rest) を参照してください。

## 設定

起動時にカレントディレクトリの `.env` を読み込みます。

| 環境変数 | 既定値 | 説明 |
| --- | --- | --- |
| `ECHO_SCHEME` | `http` | `http` または `https` |
| `ECHO_PORT` | `1323` | API の待受ポート |
| `ECHO_CERT_FILE` | `server.crt` | HTTPS 証明書のパス |
| `ECHO_KEY_FILE` | `server.key` | HTTPS 秘密鍵のパス |
| `PRINTER_DEVICE` | `/dev/usb/lp0` | プリンターデバイスのパス |
| `PRINTER_ENCODING` | `shift-jis` | `shift-jis`、`sjis`、`cp932`、または `utf-8` |
| `PRINTER_ENABLE_KANJI` | `true` | ESC/POS の漢字モードを有効にするか |
| `PRINTER_KANJI_CODE_SYSTEM` | `1` | `FS C n` で指定する漢字コード系 |
| `PRINTER_CODE_PAGE` | 未指定 | `ESC t n` で指定する単バイトコードページ（0〜255） |
| `ERROR_CORRECTION_LEVEL` | `M` | QR コードの誤り訂正レベル（`L`、`M`、`Q`、`H`） |

漢字コマンドやコードページの仕様は機種によって異なります。日本語が文字化けする場合は、プリンターのマニュアルに従って文字コード関連の設定を調整してください。

### HTTPS

開発用の自己署名証明書は次のコマンドで生成できます。

```bash
make cert
```

その後、`.env` の `ECHO_SCHEME` を `https` に設定して起動します。

## systemd で運用する

バイナリをビルドして systemd サービスを登録します。この操作には `sudo` が必要です。

```bash
make build
make install-service
```

バイナリは `/usr/local/bin/thermalprinter-go`、サービス定義は `/etc/systemd/system/thermalprinter-go.service` にインストールされます。`.env` が存在する場合は `/etc/default/thermalprinter-go` にコピーされます。

```bash
systemctl status thermalprinter-go
journalctl -u thermalprinter-go -f
```

## 開発

```bash
go test ./...
make lint
make fmt
make build
```

ホットリロードを使う場合は、Air をプロジェクト内の `bin` ディレクトリへインストールして起動します。

```bash
make install-tools
make dev
```

golangci-lint は `.golangci-lint-version` で固定されたバージョンを `make install-lint` でインストールできます。設定は `.golangci.yml` にあります。
