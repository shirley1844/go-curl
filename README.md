go-curl (fork)
==============

> **This is a fork of [andelf/go-curl](https://github.com/andelf/go-curl).**
> It fixes the deprecated multipart `Form` API and is otherwise API-compatible
> with upstream for every non-`Form` feature.
>
> **This is a fork of [andelf/go-curl](https://github.com/andelf/go-curl).**
> 本仓库是 [andelf/go-curl](https://github.com/andelf/go-curl) 的 fork，
> 修复了 multipart `Form` 使用已废弃 API 的问题，其余 API 与上游保持一致。

go-curl is a GoLang interface to [libcurl](https://curl.se/libcurl/), the
multiprotocol file transfer library. See the [upstream README](https://github.com/andelf/go-curl)
for the general documentation.

---

## Why this fork? / 为什么需要这个 fork

Upstream `go-curl` still implements multipart form upload using
**`curl_formadd()` and the `CURLFORM_*` flags**, which libcurl deprecated in
**7.56.0**. On modern distributions this produces deprecation warnings, and
breaks the build entirely under `-Werror` or on toolchains that treat the
deprecation as an error:

```
warning: 'curl_formadd' is deprecated: since 7.56.0. Use curl_mime_init()
warning: 'CURLFORM_COPYNAME' is deprecated: since 7.56.0. Use curl_mime_name()
warning: 'CURLFORM_COPYCONTENTS' is deprecated: since 7.56.0. Use curl_mime_data()
```

This is tracked upstream as
**[issue #83 "curl 7.56 deprecations"](https://github.com/andelf/go-curl/issues/83)**,
which has been open since January 2023 with no fix merged.

Additionally, the migration attempted in the previous revision of this fork
(`v0.0.1`) had a functional bug: every `Add` call created a **new** MIME
structure and overwrote `CURLOPT_MIMEPOST`, so **only the last field survived**.
`AddFromFile` was an empty function, and `NewForm` had an unusable signature
(it required a cgo-internal `*C.CURL` that callers outside the package cannot create).

This fork migrates the `Form` implementation to the modern **`curl_mime_*` API**
with correct accumulating semantics.

如果你在用这个库做 multipart 上传，并遇到 `curl_formadd is deprecated` 的编译告警/错误，
或者发现"多个字段只有最后一个生效"，那就是上面描述的问题。

---

## What changed / 改动内容

| Area | Upstream / v0.0.1 | This fork |
|---|---|---|
| Form implementation | `curl_formadd` + `CURLFORM_*` (deprecated since 7.56) | `curl_mime_*` API, **no deprecated calls** |
| Multiple `Add` calls | ❌ only the last field is sent | ✅ all fields are sent |
| `NewForm()` signature | `NewForm(*C.CURL)` (unusable outside pkg) | ✅ `NewForm()` — no arguments |
| `AddFile()` | empty / broken | ✅ uploads the real file |
| `AddFromFile()` | empty no-op function | ✅ removed (use `AddFile`) |
| Unsupported content type | `panic("not implemented")` | ✅ returns an `error` |
| Deprecation warnings | present | ✅ none |

### API / 使用方式

```go
import curl "github.com/shirley1844/go-curl"

easy := curl.EasyInit()
defer easy.Cleanup()

easy.Setopt(curl.OPT_URL, "https://example.com/upload")

form := curl.NewForm()
defer form.Free() // call before easy.Cleanup()

// bind the form to the handle, then accumulate fields
if err := easy.Setopt(curl.OPT_HTTPPOST, form); err != nil {
	log.Fatal(err)
}

form.Add("field1", "value1")                          // string
form.Add("field2", []byte{0x00, 0x01})                // raw bytes
form.AddWithType("meta", `{"a":1}`, "application/json") // explicit Content-Type
form.AddFile("upload", "/path/to/file.bin")            // upload a file
form.AddFileAs("upload2", "/path/to/file.bin", "renamed.bin") // with a custom remote filename

var body []byte
easy.Setopt(curl.OPT_WRITEFUNCTION, func(p []byte, _ interface{}) bool {
	body = append(body, p...)
	return true
})

if err := easy.Perform(); err != nil {
	log.Fatal(err)
}
```

**Important ordering** — `Setopt(OPT_HTTPPOST, form)` must be called **before**
the first `Add*`, because the MIME structure is created (and bound to the
handle) at that point:

```go
form := curl.NewForm()
form.Add("a", "b")                          // ❌ error: form is not bound
easy.Setopt(curl.OPT_HTTPPOST, form)
form.Add("a", "b")                          // ✅ ok
```

`OPT_MIMEPOST` is also accepted and behaves identically.

### Breaking changes vs. `v0.0.1`

* `NewForm()` no longer takes an argument.
* `AddFile` / `AddFileAs` return `error`.
* `AddFromFile` has been removed (it never worked); use `AddFile`.
* `Add`/`AddWithType` return an error instead of panicking on unsupported types.

If you only use non-`Form` features (perform requests, `Setopt`, callbacks,
`Getinfo`, multi handles, …), this fork is a **drop-in replacement**.

---

## Installation / 安装

```bash
go get github.com/shirley1844/go-curl@v0.0.2
```

Requirements: Go 1.21+, libcurl with CGO enabled. On Debian/Ubuntu:

```bash
sudo apt-get install -y libcurl4 libcurl4-openssl-dev
```

---

## Tests / 测试

The `Form` implementation is covered by tests that spin up a real
`multipart/form-data` server and verify the received fields:

```bash
go test -run 'TestMultipleFields|TestEmptyContent|TestAddWithType|TestAddFile|TestUnboundForm|TestUnsupportedType|TestMissingFile' -v .
```

| Test | Verifies |
|---|---|
| `TestMultipleFields` | all fields survive multiple `Add` calls (the v0.0.1 bug) |
| `TestEmptyContent` | empty values do not break later fields |
| `TestAddWithType` | explicit `Content-Type` per field |
| `TestAddFile` | file upload + custom remote filename |
| `TestUnboundForm` | `Add` before `Setopt` returns an error, does not panic |
| `TestUnsupportedType` | unsupported content returns an error |
| `TestMissingFile` | missing file returns an error |

> Note: upstream's `TestVersionInfo` may fail on hosts whose libcurl supports
> newer protocols (e.g. `mqtt`, `rtmpe`, `gophers`) that are not in its hardcoded
> allow-list. This is unrelated to the `Form` changes.

---

## License

Apache License, Version 2.0 — unchanged from upstream.
See [LICENSE](LICENSE) and [NOTICE](NOTICE).
