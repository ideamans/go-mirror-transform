# このパッケージは

あるディレクトリの配下にある、Minimatch で合致するファイルを走査し、それを別のディレクトリを起点にディレクトリの相対的な構造を保ったまま、複製を作る機構を提供します。

## 例

- images/
  - dir1/
    - file1.jpg
    - file2.png
  - file3.jpg
  - file4.txt

起点ディレクトリは input, pattern は`**/*.jpg, **/*.jpeg, **/*.png, **/*.gif`、コールバックも受け取り、マネージャーを作成する。

manager.Crawl()を実行するとディレクトリを操作してコールバックを呼び出す。

func Callback(inputPath, outputPath string) (continue bool, err error) {
// inputPath を outputPath に加工して出力する
// outputPath の dirname は確保済みであることを担保する
// continue が false の場合、その場でスキャンを終了する
// err は、Craw 自体の戻り値エラーでラップされる
return true, nil
}

以下のように WebP に変換した画像を用意できる。

- output/
  - dir1/
    - file1.jpg.webp
    - file2.png.webp
  - file3.jpg.webp

# インターフェース

- NewFilesMirror
- type FilesMirror interface

# クロール機能

- トレイリングスラッシュ
  - ディレクトリの末尾のスラッシュ、バックスラッシュの有無など適切に調整する
- 循環参照の予防
  - output が input の中にある場合や、念のため input が output の中にある場合も考慮する
- エラーハンドリングのコールバック
  - ディレクトリ走査中にエラーが発生した場合もその位置とエラーを渡してコールバックを呼び出す
  - stop bool が true なら、そこで操作を中断する
  - error が指定された場合はそれを Crawl の戻り値にラップする
- 並列処理をサポート
  - concurrency として指定
  - MaxConcurrency も指定できる。デフォルト値は CPU コア数
  - 実際の並列度は Cuncurrency と MaxConcurrency の小さい方
  - ディレクトリスキャンをしながら、concurrency で並列化されたコールバック実行を続ける
  - スキャンの並列と、コールバックの並列はそれぞれ異なる。並列スキャン群はディレクトリだけをスキャンし、ファイル処理のタスクをチャネルに送る。ファイル処理はチャネルからメッセージを受けて並列に実行する。スキャンとコールバックが同一の goroutine だと、ディレクトリの形状によってバランスよく並列化されないおそれがあるため。
  - スキャンとファイル処理の間のチャネルには 1000 件程度のバッファがあってよい。
- Graceful Shutdown
  - キャンセルがかかると、現在のコールバックが安全に終了するのを待って終了する
  - Context などを用い、Ctrl+C が押されると安全に現在の処理の終了を待ってからプロセスを終了するユースケースを想定する。
- 除外パターン
  - Except パターンも指定できる。ディレクトリやファイルがそれらに合致する場合、走査を中断する。

# ウォッチ機能

- Watch で input ディレクトリを再起的に監視し、ファイルの作成や更新があったら、対応する output ディレクトリを作成し、ファイル変換コールバックを呼び出す。
- こちらも Graceful Shutdown に対応し、Ctrl+C によるキャンセルのユースケースに対し、新規の監視通知を止めたあとで、既存の処理が安全に終了するのを待ってプロセスを終了する。

# ライブラリ

必要なライブラリは随時 go get する。

# テスト

- 並列度
  - 1, 2, 4 でテストする

# CI

## lint ジョブ

linux \* go 1.23 で golangci-lint を実行する

## coverage ジョブ

linux \* go 1.23 でテストカバレッジを取得する。

## test ジョブ

[linux, macos, windows] \* [go 1.22, go 1.22]のマトリクスでテストする。カバレッジは不要。

# README

- README.md 英語
- README.ja.md 日本語
