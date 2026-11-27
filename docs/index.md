# ga4 Provider

Manages, through the Google Analytics 4 Admin API (v1beta), accounts (by importing existing ones), properties, data retention settings,
data streams (web / Android / iOS), Measurement Protocol API secrets, custom dimensions,
custom metrics, key events, and Google Ads and Firebase links.

## 引数

- `credentials`（省略可、秘匿）: サービスアカウントの JSON キー。ファイルパスか JSON の中身そのもの。
  省略すると Application Default Credentials を使う。

資格情報の主体（サービスアカウントのメールアドレス、またはユーザー）を、GA4 のアカウントかプロパティに
「編集者」として追加しておくこと。
