---
name: extreload
description: Chrome の unpacked（フォルダから読み込んだ）拡張を、chrome://extensions の ↻ を押さずにコマンドから再読み込みする。拡張をビルドし直した後、その拡張の content script が古いタブも一緒に読み直すときに使う。
---

# extreload

`extreload` で、Chrome に「パッケージ化されていない拡張機能」として読み込んだ拡張を再読み込みする。

## コマンド

| 用途 | コマンド |
|---|---|
| 拡張を再読み込み | `extreload reload --name "<拡張の名前>"` / `--id <拡張ID>` |
| ついでにタブも読み直す | `extreload reload --name "<名前>" --tabs "http://localhost:8081/*"`（`--tabs` は複数可） |
| 対象を1つの Chrome に絞る | `--profile <ラベル>` |
| 読み込まれている unpacked 拡張の一覧 | `extreload list` |
| サーバと接続中の Chrome | `extreload status` |

## 注意点

- 対象の Chrome に **extreload helper** 拡張（このリポジトリの `extension/`）が読み込まれている必要がある。入っていなければ `reload` は「no Chrome is connected」で失敗する。ユーザーに chrome://extensions →「パッケージ化されていない拡張機能を読み込む」で `extension/` を選んでもらう。
- サーバ（`extreload serve`）は最初の `reload` / `list` で自動的に裏で立ち上がる。ログは `~/.extreload/serve.log`。
- helper が眠っていると、つながるまで最大 30 秒ほど待つ（`--wait` で変更可）。
- 拡張を読み直すと、既存のタブにあった content script は拡張と切れる。その拡張がページに部品を差し込んでいるなら、`--tabs` でそのページも読み直す。
- 名前は chrome://extensions に出ている名前と完全一致（大文字小文字は区別しない）。分からなければ先に `extreload list`。
- helper が複数の Chrome（プロファイル）にあると、`--profile` を付けない限り全部で読み直す。
