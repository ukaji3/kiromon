# きろもん kiromon 

CLIツール（kiro-cliなど）の状態を監視し、入力待ち/実行中を検出して通知するユーティリティ。

Kiro にタスクを依頼して完了した際に、読み上げソフトなどでユーザーにお知らせするような用途を想定。

実装は、ユーザーのプロンプト入力待ちになったら、定義した子プロセスに引数を与え起動する。


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

### 基本（スタンドアロンモード）

コマンドの実行と監視を1プロセスで行います。状態変化時に通知コマンドを実行できます。

```bash
# notify-sendで通知（終了時のみ）
kiromon -c notify-send -me "タスク完了" kiro-cli chat

# 読み上げソフトと連動（開始・終了両方）
kiromon -c voicevox-speak-standalone -ms "{time}、タスクを開始したのだ" -me "{time}、タスクを終了したのだ。処理時間は、{duration}だったのだ。" kiro-cli chat -a -r

# macOSのsayコマンドで読み上げ（開始・終了両方）
kiromon -c say -ms "{time}、タスクを開始したのだ" -me "{time}、タスクを終了したのだ。処理時間は、{duration}だったのだ。" kiro-cli chat -a -r

# カスタムプロンプトパターン
kiromon -c notify-send -me "完了" -r '> ?$'
```

#### オプション

| オプション | 説明 |
|-----------|------|
| `-c <cmd>` | 状態変化時に実行するコマンド（必須） |
| `-ms <msg>` | 開始時（running状態）のメッセージ。省略時は開始時の通知なし |
| `-me <msg>` | 終了時（waiting状態）のメッセージ。省略時は終了時の通知なし |
| `-r <regex>` | カスタムプロンプトパターン（デフォルト: `> ?$`） |
| `-log <path>` | ログファイルパス（デフォルト: `kiromon.log`） |
| `--` | これ以降を監視対象コマンドとして扱う（オプションの区切り） |

#### プレースホルダ

メッセージ内で以下のプレースホルダが使用できます：

| プレースホルダ | 説明 |
|---------------|------|
| `{time}` | 現在時刻（xx時xx分xx秒形式、0の部分は省略） |
| `{duration}` | タスク処理時間（xx時間xx分xx秒形式、0の部分は省略） |

```bash
# 処理時間を通知
kiromon -c notify-send -me "タスク完了 処理時間: {duration}" kiro-cli chat

# 時刻と処理時間を両方表示
kiromon -c say -me "{time} タスク完了。{duration}かかりました" kiro-cli chat
```

監視対象コマンドが `-` で始まるオプションを持つ場合は `--` で区切ります：

```bash
kiromon -c notify-send -ms "開始" -me "完了" -- some-cmd --verbose --debug
```

#### 動作

- 監視ログは `kiromon.log` に出力（`-log` で変更可能）
- 通知コマンドの出力も同ファイルに記録
- ターミナルにはコマンドの出力のみ表示
- 状態変化後1秒間安定してから通知（デバウンス処理）

### 通知なしで実行

単純にコマンドを監視付きで実行（ステータスファイルのみ出力）:

```bash
kiromon kiro-cli chat
```

---

## 外部監視（副機能）

別プロセスから状態を監視する機能です。複数インスタンスの一括監視などに使用します。

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
kiromon -s kiro-cli -d -c notify-send -ms "タスク開始" -me "タスク完了"

# 終了時のみ通知
kiromon -s kiro-cli -d -c notify-send -me "タスク完了"

# ポーリング間隔を変更（デフォルト: 2秒）
kiromon -s kiro-cli -d -i 5

# カスタムプロンプトパターン
kiromon -s kiro-cli -d -r '> ?$' -me "完了" -c notify-send
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

**注意**: スタンドアロンモード使用時は、同じコマンドに対してデーモンモード（`-s -d`）を同時に実行しないでください。両方から通知が発火し、コマンドが2回呼ばれます。

---

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

## 設定ファイル

`~/.config/kiromon/config.yaml` でデフォルト設定を管理できます（オプション）。

サンプル設定ファイル: [docs/config.example.yaml](docs/config.example.yaml)

```yaml
# デフォルトの通知コマンド
default_command: notify-send

# プロンプト検出パターン（複数指定可）
prompt_patterns:
  - '> ?$'
  - '\$ $'

# ログファイルパス
log_path: /var/log/kiromon.log

# コマンドごとのプリセット
presets:
  kiro-cli:
    start_msg: "{time}、タスクを開始したのだ"
    end_msg: "{time}、タスクを終了したのだ。処理時間は、{duration}だったのだ。"
```

設定ファイルが存在しない場合は、組み込みのデフォルト値が使用されます。
コマンドラインオプションは設定ファイルより優先されます。

## ライセンス

MIT License
