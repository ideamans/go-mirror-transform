# go-files-mirror

ディレクトリ構造を保持しながら、あるディレクトリから別のディレクトリへファイルをミラーリングするGoパッケージです。パターンマッチング、並行処理、ファイル監視機能をサポートしています。

## 機能

- **パターンベースのファイル選択** - glob形式（minimatchスタイル）によるパターンマッチング
- **並行処理** - 設定可能な並列度での処理
- **ディレクトリ構造の保持** - 出力ディレクトリでも元の構造を維持
- **ファイル監視** - リアルタイム同期のための監視機能
- **グレースフルシャットダウン** - コンテキストキャンセルによる安全な終了
- **循環参照の防止** - 無限ループを回避する安全機構
- **カスタマイズ可能なエラーハンドリング** - コールバックによる柔軟なエラー処理

## インストール

```bash
go get github.com/ideamans/go-files-mirror
```

## 使い方

### グレースフルシャットダウンの基本例

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    mirrortransform "github.com/ideamans/go-mirror-transform"
)

func main() {
    config := mirrortransform.Config{
        InputDir:  "images",
        OutputDir: "output",
        Patterns:  []string{"**/*.jpg", "**/*.png", "**/*.gif"},
        Concurrency: 4,
        FileCallback: func(inputPath, outputPath string) (bool, error) {
            // ファイルを処理（例：WebPに変換）
            // outputPathのディレクトリは存在が保証されています
            log.Printf("処理中: %s -> %s\n", inputPath, outputPath)
            return true, nil
        },
    }

    mt, err := mirrortransform.NewMirrorTransform(&config)
    if err != nil {
        log.Fatal(err)
    }

    // キャンセル可能なコンテキストを作成
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Ctrl+Cでグレースフルシャットダウン
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigCh
        log.Println("シャットダウンシグナルを受信しました。安全に停止しています...")
        cancel()
    }()

    // コンテキスト付きでクロールを実行
    if err := mt.Crawl(ctx); err != nil {
        if err == context.Canceled {
            log.Println("クロールが安全に停止しました")
        } else {
            log.Fatal(err)
        }
    }
}
```

### グレースフルシャットダウン対応のウォッチモード

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    mirrortransform "github.com/ideamans/go-mirror-transform"
)

func main() {
    config := mirrortransform.Config{
        InputDir:  "images",
        OutputDir: "output",
        Patterns:  []string{"**/*.jpg", "**/*.png", "**/*.gif"},
        Concurrency: 4,
        FileCallback: func(inputPath, outputPath string) (bool, error) {
            log.Printf("処理中: %s -> %s\n", inputPath, outputPath)
            // ファイルを処理（例：WebPに変換）
            return true, nil
        },
        ErrorCallback: func(path string, err error) (bool, error) {
            log.Printf("エラー %s: %v\n", path, err)
            return false, nil // 処理を継続
        },
    }

    mt, err := mirrortransform.NewMirrorTransform(&config)
    if err != nil {
        log.Fatal(err)
    }

    // キャンセル可能なコンテキストを作成
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // グレースフルシャットダウンの処理
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigCh
        log.Println("シャットダウンシグナルを受信しました。監視を停止しています...")
        cancel()
    }()

    log.Println("ファイルの変更を監視しています。Ctrl+Cで停止します。")
    
    // 監視を開始（コンテキストがキャンセルされるまでブロック）
    if err := mt.Watch(ctx); err != nil {
        if err == context.Canceled {
            log.Println("監視が安全に停止しました")
        } else {
            log.Fatal(err)
        }
    }
}
```

### タイムアウト付きクロールとウォッチの組み合わせ

```go
package main

import (
    "context"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"
    mirrortransform "github.com/ideamans/go-mirror-transform"
)

func main() {
    var (
        watchMode bool
        timeout   time.Duration
    )
    flag.BoolVar(&watchMode, "watch", false, "ウォッチモードを有効化")
    flag.DurationVar(&timeout, "timeout", 60*time.Second, "処理のタイムアウト時間")
    flag.Parse()

    config := mirrortransform.Config{
        InputDir:    "images",
        OutputDir:   "output",
        Patterns:    []string{"**/*.jpg", "**/*.png", "**/*.gif"},
        Concurrency: 4,
        FileCallback: func(inputPath, outputPath string) (bool, error) {
            log.Printf("処理中: %s\n", inputPath)
            // ここに処理ロジックを記述
            return true, nil
        },
        ErrorCallback: func(path string, err error) (bool, error) {
            log.Printf("エラー %s: %v\n", path, err)
            return false, nil // 処理を継続
        },
    }

    mt, err := mirrortransform.NewMirrorTransform(&config)
    if err != nil {
        log.Fatal(err)
    }

    // タイムアウト付きのベースコンテキストを作成
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    // グレースフルシャットダウンのためのシグナルハンドリング
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    
    // シグナルを処理する別のゴルーチンを作成
    go func() {
        select {
        case <-sigCh:
            log.Println("割り込みシグナルを受信しました。シャットダウンしています...")
            cancel()
        case <-ctx.Done():
            // タイムアウトまたは他の理由でコンテキストがキャンセルされた
        }
    }()

    // 処理を開始
    startTime := time.Now()
    if watchMode {
        log.Printf("ウォッチモードで起動しました。%v 後にタイムアウトします。Ctrl+Cで停止できます。", timeout)
        err = mt.Watch(ctx)
    } else {
        log.Printf("クロールを開始しました。%v 後にタイムアウトします。Ctrl+Cで停止できます。", timeout)
        err = mt.Crawl(ctx)
    }

    // 異なるタイプのコンテキストキャンセルを処理
    duration := time.Since(startTime)
    switch {
    case err == context.DeadlineExceeded:
        log.Printf("%v 経過後にタイムアウトしました", duration)
    case err == context.Canceled:
        log.Printf("%v 経過後にキャンセルされました", duration)
    case err != nil:
        log.Fatalf("操作が失敗しました: %v", err)
    default:
        log.Printf("操作が %v で正常に完了しました", duration)
    }
}
```

## 設定

### Config フィールド

- `InputDir` (string, 必須): スキャン対象のルートディレクトリ
- `OutputDir` (string, 必須): 処理済みファイルを配置するルートディレクトリ
- `Patterns` ([]string, 必須): ファイルにマッチするglobパターン（例：`**/*.jpg`）
- `ExcludePatterns` ([]string): 除外するファイル/ディレクトリのパターン
- `Concurrency` (int): 並列ファイル処理数
- `MaxConcurrency` (int): 最大並列度（デフォルトはCPU数）
- `FileCallback` (func, 必須): マッチしたファイルごとに呼ばれる関数
- `ErrorCallback` (func): 走査中にエラーが発生した際に呼ばれる関数

## コールバック関数

### FileCallback

`FileCallback`関数は、パターンにマッチした各ファイルに対して呼び出されます。ファイルの処理方法を完全に制御できます。

```go
type FileCallback func(inputPath, outputPath string) (continueProcessing bool, err error)
```

**引数:**
- `inputPath`: ソースファイルへの完全な絶対パス
- `outputPath`: 推奨される出力パス（入力からのディレクトリ構造を維持）。必要に応じてこのパスを変更できます。

**戻り値:**
- `continueProcessing`: `false`の場合、クロール/ウォッチ操作全体が停止します
- `err`: nilでない場合、エラーハンドリングがトリガーされ、処理が停止する可能性があります

**例1: テキストファイルを大文字に変換**
```go
FileCallback: func(inputPath, outputPath string) (bool, error) {
    // 入力ファイルを読み込む
    content, err := os.ReadFile(inputPath)
    if err != nil {
        return false, fmt.Errorf("ファイルの読み込みに失敗: %w", err)
    }
    
    // 内容を大文字に変換
    upperContent := strings.ToUpper(string(content))
    
    // 出力パスに.uc拡張子を追加
    outputPath = outputPath + ".uc"
    
    // 変換した内容を書き込む
    err = os.WriteFile(outputPath, []byte(upperContent), 0644)
    if err != nil {
        return false, fmt.Errorf("ファイルの書き込みに失敗: %w", err)
    }
    
    log.Printf("変換完了: %s -> %s", inputPath, outputPath)
    return true, nil // 他のファイルの処理を継続
}
```

**例2: カスタム命名での画像変換**
```go
FileCallback: func(inputPath, outputPath string) (bool, error) {
    // 拡張子を.webpに変更
    outputPath = strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".webp"
    
    // 画像変換ロジックをここに記述
    // convertToWebP(inputPath, outputPath)
    
    return true, nil
}
```

**例3: 条件付き処理**
```go
FileCallback: func(inputPath, outputPath string) (bool, error) {
    // 10MBより大きいファイルをスキップ
    info, err := os.Stat(inputPath)
    if err != nil {
        return false, err
    }
    
    if info.Size() > 10*1024*1024 {
        log.Printf("大きなファイルをスキップ: %s (%d バイト)", inputPath, info.Size())
        return true, nil // 次のファイルに進む
    }
    
    // ファイルを処理
    return true, processFile(inputPath, outputPath)
}
```

### ErrorCallback

`ErrorCallback`はディレクトリ走査中のエラーを処理し、エラーからの回復を制御できます。

```go
type ErrorCallback func(path string, err error) (stop bool, retErr error)
```

**引数:**
- `path`: エラーが発生したパス
- `err`: 発生したエラー

**戻り値:**
- `stop`: `true`の場合、操作全体が停止します
- `retErr`: nilでない場合、このエラーがCrawl/Watchから返されます（ラップされた形で）

**例: エラーをログに記録して処理を継続**
```go
ErrorCallback: func(path string, err error) (bool, error) {
    // エラーをログに記録
    log.Printf("エラー発生 %s: %v", path, err)
    
    // 権限エラーかチェック
    if os.IsPermission(err) {
        log.Printf("権限がないためスキップ: %s", path)
        return false, nil // 処理を継続
    }
    
    // その他のエラーでは停止
    return true, fmt.Errorf("致命的なエラー: %w", err)
}
```

## パターン構文

パターンはminimatchスタイルのglob構文を使用します：
- `*` - パス区切り文字以外の任意の文字列にマッチ
- `**` - ゼロ個以上のディレクトリにマッチ
- `?` - 任意の1文字にマッチ
- `[abc]` - セット内の任意の文字にマッチ
- `{a,b,c}` - いずれかの選択肢にマッチ

例：
- `**/*.jpg` - 任意のサブディレクトリ内のすべてのJPGファイル
- `images/**/*.{jpg,png}` - images/配下のJPGとPNGファイル
- `**/thumb_*.jpg` - "thumb_"で始まるJPGファイル

## 並行処理

このパッケージは2つのレベルの並列処理を使用します：
1. ディレクトリスキャンは並列のgoroutineで実行
2. ファイル処理は別のワーカープールで実行

実際の並列度は `min(Concurrency, MaxConcurrency)` となります。この設計により、ディレクトリ構造に関わらず効率的な処理を実現します。

## 安全機能

- **循環参照の防止**: 出力ディレクトリが入力ディレクトリ内にある場合を自動検出して防止
- **グレースフルシャットダウン**: 終了前に進行中のファイル操作の完了を待機
- **ディレクトリ作成**: 必要に応じて出力ディレクトリを自動作成
- **パスクリーニング**: 末尾のスラッシュやパス区切り文字を適切に処理

## ライセンス

MIT License