# ComposePilot

ComposePilot は、単一 Docker ホスト上で GitHub 上の Docker Compose アプリケーションを管理するための、Go 製シングルバイナリのコントロールプレーンです。

![ComposePilot screenshot](docs/screenshot.png)

## 主な機能
- リポジトリ、ブランチ、deploy key、compose ファイル、環境変数の登録
- サービスごとの deploy key を使った SSH clone / pull
- `docker compose build`、`up -d`、`restart` の実行
- コンテナ一覧、ログの検索・追尾、ブラウザ経由の `docker exec`
- Docker network の一覧表示と作成
- SQLite による設定・履歴保存と、AES-GCM による秘密情報の暗号化

## ローカルで起動する
1. `.env.example` を `.env` にコピーします。
2. `COMPOSEPILOT_MASTER_KEY` または `COMPOSEPILOT_MASTER_KEY_FILE` を設定します。
3. 次のコマンドで起動します。

```bash
go run ./cmd/composepilot -listen :8080 -data-dir ./data -workspace ./workspace
```

ComposePilot は、リポジトリ直下に `.env` があれば自動で読み込みます。

## GitHub Releases から一発でインストールする
Linux では、次のワンライナーでインストールできます。

```bash
curl -fsSL https://raw.githubusercontent.com/shunsukeaihara/ComposePilot/main/install.sh | sudo sh
```

このスクリプトは次を行います。
- 現在の OS / arch に合う最新 Release バイナリをダウンロード
- `composepilot` を `/usr/local/bin` に配置
- 必要なら master key を生成
- `composepilot` ユーザーがなければ作成
- `docker` グループが存在すれば `composepilot` を追加
- `systemd` サービスを登録して起動

バージョンや listen アドレスを固定したい場合:

```bash
curl -fsSL https://raw.githubusercontent.com/shunsukeaihara/ComposePilot/main/install.sh | \
  sudo COMPOSEPILOT_VERSION=v0.1.0 COMPOSEPILOT_LISTEN=:9090 sh
```

installer script で指定できる変数と既定値:

| 変数 | 既定値 |
| --- | --- |
| `COMPOSEPILOT_VERSION` | 最新 Release tag |
| `COMPOSEPILOT_LISTEN` | `:8080` |
| `COMPOSEPILOT_BIN_DIR` | `/usr/local/bin` |
| `COMPOSEPILOT_CONFIG_DIR` | `/etc/composepilot` |
| `COMPOSEPILOT_DATA_DIR` | `/var/lib/composepilot` |
| `COMPOSEPILOT_WORKSPACE_DIR` | `${COMPOSEPILOT_DATA_DIR}/workspace` |

補足:
- SQLite の DB パスは `${COMPOSEPILOT_DATA_DIR}/composepilot.db` 固定です
- master key file は `${COMPOSEPILOT_CONFIG_DIR}/master_key` 固定です
- environment file は `${COMPOSEPILOT_CONFIG_DIR}/composepilot.env` 固定です
- `install.sh` は現状 Linux 専用です

## air で自動再起動する
1. `air` をインストールします。

```bash
go install github.com/air-verse/air@latest
```

2. `.env` を用意し、必要なら `dev/master_key` を作成します。
3. リポジトリ直下で次を実行します。

```bash
air
```

`.air.toml` は `.go` と埋め込み UI の `.html` 変更を監視し、ComposePilot を自動で再ビルド・再起動します。

## 配布用バイナリを生成する
付属の `Makefile` で `dist/` 配下にクロスコンパイル済みバイナリを生成できます。

```bash
make dist VERSION=0.1.0
```

使えるターゲット:
- `make dist-linux`
- `make dist-windows`
- `make dist-darwin`
- `make dist-linux-amd64`
- `make dist-linux-arm64`
- `make dist-windows-amd64`
- `make dist-darwin-amd64`
- `make dist-darwin-arm64`

生成されたバイナリは次のようにバージョン表示できます。

```bash
./dist/composepilot-linux-amd64 -version
```

## GitHub Releases に配布バイナリを公開する
`v0.1.0` のようなタグを push すると、GitHub Actions が各OS向けの配布アーカイブをビルドして、同名の GitHub Release に添付します。

```bash
git tag v0.1.0
git push origin v0.1.0
```

installer script は既定で最新 Release を参照し、`COMPOSEPILOT_VERSION` を指定した場合はそのバージョンを取得します。

## 本番環境での鍵の渡し方
推奨順は以下です。
1. `COMPOSEPILOT_MASTER_KEY_FILE` を使い、root 所有のファイルを参照する
2. `systemd` の `EnvironmentFile=` 経由で渡す
3. 生の鍵文字列を起動コマンドへ直接書かない

サンプル:
- `deploy/systemd/composepilot.service`
- `deploy/systemd/composepilot.env.example`

セットアップ例:

```bash
sudo install -d -m 700 /etc/composepilot
sudo install -m 600 deploy/systemd/composepilot.env.example /etc/composepilot/composepilot.env
sudo sh -c 'printf "%s\n" "<base64-key>" > /etc/composepilot/master_key'
sudo chmod 600 /etc/composepilot/master_key
sudo install -m 644 deploy/systemd/composepilot.service /etc/systemd/system/composepilot.service
sudo systemctl daemon-reload
sudo systemctl enable --now composepilot
```

## 補足
- ComposePilot 実行ユーザーから `docker` / `docker compose` が使える必要があります。
- Docker ソケットへアクセスできる権限が必要です。
- 埋め込み UI は `internal/http/templates` にあります。
