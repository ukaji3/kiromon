# きろもん kiromon 

CLIツール（kiro-cliなど）の状態を監視し、入力待ち/実行中を検出して通知するユーティリティ。

## サポートOS

| OS | サポート | 備考 |
|----|---------|------|
| Linux | ✅ | `$XDG_RUNTIME_DIR` を使用 |
| macOS | ✅ | `$TMPDIR` を使用 |
| Windows | ❌ | PTY非対応 |

## インストール

```bash
go install github.com/ukaji3/kiromon@latest
```

または、ソースからビルド:

```bash
git clone https://github.com/ukaji3/kiromon.git
cd kiromon
go build -o kiromon .
```

## 使い方

### コマンドを監視付きで実行

```bash
kiromon kiro-cli chat
```

### 状態を確認

```bash
# 全インスタンスを表示
kiromon -s kiro-cli

# 特定PIDのみ表示
kiromon -s kiro-cli -p 12345
```

### デーモンモードで監視

状態変化時に外部コマンドを実行:

```bash
# notify-sendで通知
kiromon -s kiro-cli -d -c notify-send

# 特定PIDのみ監視
kiromon -s kiro-cli -d -p 12345 -c notify-send

# カスタムメッセージ
kiromon -s kiro-cli -d -c notify-send -m "タスク開始" "タスク完了"

# 読み上げソフトと連動する
kiromon -s kiro-cli -d -c voicevox-speak-standalone -m "タスクを開始したのだ" "タスクを終了したのだ"

# ポーリング間隔を変更（デフォルト: 2秒）
kiromon -s kiro-cli -d -i 5

# カスタムプロンプトパターン
kiromon -s kiro-cli -d -r '> ?$'
```$'
```

### 監視中プロセス一覧

```bash
kiromon -l
```

出力例:
```
Monitored processes:
----------------------------------------------------------------------
⏳ vim                  PID:12345    idle: 2.3s
📦 kiro-cli (3 instances)
   🔄 PID:23456    idle: 1.2s
   ⏳ PID:34567    idle: 5.0s
   🔄 PID:45678    idle: 0.5s
```

## 状態

| 状態 | 説明 |
|------|------|
| 🔄 running | コマンド実行中 |
| ⏳ waiting | 入力待ち（プロンプト検出） |
| ⏹ stopped | 終了 |

## プロセス間通信

kiromonはファイルベースのIPCを使用して、ラッパープロセスとモニタープロセス間で状態を共有します。

### アーキテクチャ

```
┌─────────────────┐         ┌──────────────────┐
│  kiromon        │         │  kiromon -s -d   │
│  (wrapper)      │         │  (monitor)       │
│                 │         │                  │
│  ┌───────────┐  │  JSON   │  ┌────────────┐  │
│  │ PTY       │──┼────────►│  │ File Watch │  │
│  │ Capture   │  │  file   │  │ + Polling  │  │
│  └───────────┘  │         │  └────────────┘  │
│        │        │         │        │         │
│        ▼        │         │        ▼         │
│  ┌───────────┐  │         │  ┌────────────┐  │
│  │ State     │  │         │  │ Callback   │  │
│  │ Detection │  │         │  │ Command    │  │
│  └───────────┘  │         │  └────────────┘  │
└─────────────────┘         └──────────────────┘
```

### ステータスファイル

状態は以下の場所にJSONで保存されます:

- Linux: `$XDG_RUNTIME_DIR/kiromon/<name>-<pid>.json`
- macOS/その他: `$TMPDIR/kiromon-<uid>/<name>-<pid>.json`

同じコマンドを複数起動した場合、PID付きのファイル名で区別されます。

### ファイルロック

- 書き込み: アトミック書き込み（一時ファイル→リネーム）で競合を防止
- 読み取り: `flock(LOCK_SH)` で共有ロックを取得

### ステータスJSON形式

```json
{
  "state": "waiting",
  "command": "kiro-cli chat",
  "pid": 12345,
  "start_time": "2024-01-01T12:00:00Z",
  "updated_at": "2024-01-01T12:01:00Z",
  "last_lines": ["output line 1", "output line 2"],
  "last_line": "> ",
  "prompt_matched": true,
  "idle_seconds": 5.2
}
```

| フィールド | 説明 |
|-----------|------|
| `state` | `running`, `waiting`, `stopped` |
| `command` | 実行中のコマンド |
| `pid` | プロセスID |
| `last_lines` | 直近20行の出力 |
| `last_line` | 現在の行（プロンプト検出用） |
| `prompt_matched` | プロンプトパターンにマッチしたか |
| `idle_seconds` | 最後のI/Oからの経過秒数 |

### 外部連携

ステータスファイルを読み取ることで、他のツールからも状態を取得できます:

```bash
# シェルスクリプトから状態を取得（PID付きファイル名）
cat $XDG_RUNTIME_DIR/kiromon/kiro-cli-12345.json | jq .state

# 全kiro-cliインスタンスの状態を取得
for f in $XDG_RUNTIME_DIR/kiromon/kiro-cli-*.json; do
  echo "$(basename $f): $(jq -r .state $f)"
done

# 入力待ちになったら通知
while true; do
  state=$(cat $XDG_RUNTIME_DIR/kiromon/kiro-cli-*.json 2>/dev/null | jq -r .state | head -1)
  if [ "$state" = "waiting" ]; then
    notify-send "kiro-cli is waiting"
    break
  fi
  sleep 1
done
```

### クリーンアップ

- プロセス終了時にステータスファイルは自動削除
- 24時間以上古いファイルは起動時に自動クリーンアップ
- 死んだプロセスのファイルも起動時に削除

## ライセンス

MIT License
