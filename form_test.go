package curl_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	curl "github.com/shirley1844/go-curl"
)

// 本地测试服务器：解析 multipart 并回显字段
func newServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Errorf("服务端解析 multipart 失败: %v", err)
			w.WriteHeader(400)
			return
		}
		out := map[string]interface{}{}
		for k, v := range r.MultipartForm.Value {
			out[k] = strings.Join(v, ",")
		}
		for k, v := range r.MultipartForm.File {
			if len(v) > 0 {
				f, _ := v[0].Open()
				buf := make([]byte, v[0].Size)
				f.Read(buf)
				f.Close()
				out[k] = string(buf)
				out[k+"__filename"] = v[0].Filename
				out[k+"__type"] = v[0].Header.Get("Content-Type")
			}
		}
		out["__content_type"] = strings.Split(r.Header.Get("Content-Type"), ";")[0]
		json.NewEncoder(w).Encode(out)
	}))
}

// 标准用法：先创建 easy + form 并绑定，再 Add 字段，最后 Perform
type client struct {
	easy *curl.CURL
	form *curl.Form
}

func newClient(t *testing.T, url string) *client {
	easy := curl.EasyInit()
	form := curl.NewForm()
	if err := easy.Setopt(curl.OPT_HTTPPOST, form); err != nil {
		t.Fatalf("绑定 form 失败: %v", err)
	}
	easy.Setopt(curl.OPT_URL, url)
	return &client{easy: easy, form: form}
}

func (c *client) close() {
	c.form.Free()
	c.easy.Cleanup()
}

func (c *client) do(t *testing.T) map[string]interface{} {
	var body []byte
	c.easy.Setopt(curl.OPT_WRITEFUNCTION, func(p []byte, _ interface{}) bool {
		body = append(body, p...)
		return true
	})
	if err := c.easy.Perform(); err != nil {
		t.Fatalf("Perform 失败: %v", err)
	}
	var out map[string]interface{}
	json.Unmarshal(body, &out)
	return out
}

// 核心回归测试：多次 Add 必须全部生效（旧实现只有最后一个字段生效）
func TestMultipleFields(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL)
	defer c.close()

	if err := c.form.Add("field1", "value1"); err != nil {
		t.Fatal(err)
	}
	if err := c.form.Add("field2", "value2"); err != nil {
		t.Fatal(err)
	}
	if err := c.form.Add("field3", []byte("value3")); err != nil {
		t.Fatal(err)
	}

	got := c.do(t)
	t.Logf("服务端收到: %#v", got)

	if got["__content_type"] != "multipart/form-data" {
		t.Errorf("Content-Type 应为 multipart/form-data，实际 %v", got["__content_type"])
	}
	for k, want := range map[string]string{"field1": "value1", "field2": "value2", "field3": "value3"} {
		if got[k] != want {
			t.Errorf("字段 %s 期望 %q 实际 %v", k, want, got[k])
		}
	}
}

// 空内容字段不应影响后续字段
func TestEmptyContent(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL)
	defer c.close()

	c.form.Add("empty", "")
	c.form.Add("after", "ok")

	got := c.do(t)
	t.Logf("服务端收到: %#v", got)
	if got["after"] != "ok" {
		t.Errorf("空字段之后的内容丢失: %v", got)
	}
}

// AddWithType 指定 Content-Type
func TestAddWithType(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL)
	defer c.close()

	c.form.AddWithType("json", `{"a":1}`, "application/json")

	got := c.do(t)
	t.Logf("服务端收到: %#v", got)
	if got["json"] != `{"a":1}` {
		t.Errorf("json 字段内容错误: %v", got["json"])
	}
}

// AddFile / AddFileAs 上传文件
func TestAddFile(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	dir := t.TempDir()
	fp := filepath.Join(dir, "hello.txt")
	os.WriteFile(fp, []byte("file-content-123"), 0644)

	c := newClient(t, srv.URL)
	defer c.close()

	c.form.Add("meta", "x")
	if err := c.form.AddFile("upload", fp); err != nil {
		t.Fatal(err)
	}
	if err := c.form.AddFileAs("upload2", fp, "renamed.txt"); err != nil {
		t.Fatal(err)
	}

	got := c.do(t)
	t.Logf("服务端收到: %#v", got)
	if got["upload"] != "file-content-123" {
		t.Errorf("文件内容错误: %v", got["upload"])
	}
	if got["upload__filename"] != "hello.txt" {
		t.Errorf("文件名错误: %v", got["upload__filename"])
	}
	if got["upload2__filename"] != "renamed.txt" {
		t.Errorf("重命名后文件名错误: %v", got["upload2__filename"])
	}
}

// 未绑定时调用 Add 应报错而不是 panic
func TestUnboundForm(t *testing.T) {
	form := curl.NewForm()
	if err := form.Add("a", "b"); err == nil {
		t.Error("未绑定句柄时 Add 应返回错误")
	}
}

// 不支持的 content 类型应返回错误而不是 panic
func TestUnsupportedType(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL)
	defer c.close()

	if err := c.form.Add("bad", 12345); err == nil {
		t.Error("不支持的 content 类型应返回错误")
	}
}

// 文件不存在应返回错误
func TestMissingFile(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL)
	defer c.close()

	if err := c.form.AddFile("f", "/nonexistent/path.bin"); err == nil {
		t.Error("不存在的文件应返回错误")
	}
}
