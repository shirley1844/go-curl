package curl

/*
#include <stdlib.h>
#include <curl/curl.h>
#include "callback.h"
#include "compat.h"

static CURLcode curl_easy_setopt_long(CURL *handle, CURLoption option, long parameter) {
  return curl_easy_setopt(handle, option, parameter);
}
static CURLcode curl_easy_setopt_string(CURL *handle, CURLoption option, char *parameter) {
  return curl_easy_setopt(handle, option, parameter);
}
static CURLcode curl_easy_setopt_slist(CURL *handle, CURLoption option, struct curl_slist *parameter) {
  return curl_easy_setopt(handle, option, parameter);
}
static CURLcode curl_easy_setopt_pointer(CURL *handle, CURLoption option, void *parameter) {
  return curl_easy_setopt(handle, option, parameter);
}
static CURLcode curl_easy_setopt_off_t(CURL *handle, CURLoption option, off_t parameter) {
  return curl_easy_setopt(handle, option, parameter);
}

static CURLcode curl_easy_getinfo_string(CURL *curl, CURLINFO info, char **p) {
 return curl_easy_getinfo(curl, info, p);
}
static CURLcode curl_easy_getinfo_long(CURL *curl, CURLINFO info, long *p) {
 return curl_easy_getinfo(curl, info, p);
}
static CURLcode curl_easy_getinfo_double(CURL *curl, CURLINFO info, double *p) {
 return curl_easy_getinfo(curl, info, p);
}
static CURLcode curl_easy_getinfo_slist(CURL *curl, CURLINFO info, struct curl_slist **p) {
 return curl_easy_getinfo(curl, info, p);
}

// curl_mime_create 创建一个新的 MIME 表单结构并挂到句柄上。
// 返回 NULL 表示内存不足（CURLE_OUT_OF_MEMORY）。
// 注意：生命周期由调用方（Go 侧 Form）管理，不在此处释放。
static curl_mime *curl_mime_create(CURL *curl) {
    curl_mime *form = curl_mime_init(curl);
    if (!form)
        return NULL;
    if (curl_easy_setopt(curl, CURLOPT_MIMEPOST, form) != CURLE_OK) {
        curl_mime_free(form);
        return NULL;
    }
    return form;
}

// curl_mime_free_form 释放 MIME 表单结构。
static void curl_mime_free_form(curl_mime *form) {
    if (form)
        curl_mime_free(form);
}

// curl_mime_add_part_data 追加一个内容为原始字节的表单字段。
static CURLcode curl_mime_add_part_data(curl_mime *form, const char *name,
                                        const char *content, size_t length,
                                        const char *type) {
    curl_mimepart *field;

    if (!form || !name)
        return CURLE_BAD_FUNCTION_ARGUMENT;

    field = curl_mime_addpart(form);
    if (!field)
        return CURLE_OUT_OF_MEMORY;

    if (curl_mime_name(field, name) != CURLE_OK)
        return CURLE_OUT_OF_MEMORY;
    if (curl_mime_data(field, content, length) != CURLE_OK)
        return CURLE_OUT_OF_MEMORY;
    if (type && type[0] && curl_mime_type(field, type) != CURLE_OK)
        return CURLE_OUT_OF_MEMORY;

    return CURLE_OK;
}

// curl_mime_add_part_file 追加一个以文件内容为数据的表单字段。
static CURLcode curl_mime_add_part_file(curl_mime *form, const char *name,
                                        const char *filename,
                                        const char *remote_filename,
                                        const char *type) {
    curl_mimepart *field;

    if (!form || !name || !filename)
        return CURLE_BAD_FUNCTION_ARGUMENT;

    field = curl_mime_addpart(form);
    if (!field)
        return CURLE_OUT_OF_MEMORY;

    if (curl_mime_name(field, name) != CURLE_OK)
        return CURLE_OUT_OF_MEMORY;
    if (curl_mime_filedata(field, filename) != CURLE_OK)
        return CURLE_READ_ERROR;
    if (remote_filename && remote_filename[0] &&
        curl_mime_filename(field, remote_filename) != CURLE_OK)
        return CURLE_OUT_OF_MEMORY;
    if (type && type[0] && curl_mime_type(field, type) != CURLE_OK)
        return CURLE_OUT_OF_MEMORY;

    return CURLE_OK;
}


 // TODO: support multi file

*/
import "C"

import (
	"errors"
	"fmt"
	"mime"
	"path"
	"sync"
	"unsafe"
)

type CurlInfo C.CURLINFO
type CurlError C.CURLcode

type CurlString *C.char

func NewCurlString(s string) CurlString {
	return CurlString(unsafe.Pointer(C.CString(s)))
}

func FreeCurlString(s CurlString) {
	C.free(unsafe.Pointer(s))
}

func (e CurlError) Error() string {
	// ret is const char*, no need to free
	ret := C.curl_easy_strerror(C.CURLcode(e))
	return fmt.Sprintf("curl: %s", C.GoString(ret))
}

func newCurlError(errno C.CURLcode) error {
	if errno == C.CURLE_OK { // if nothing wrong
		return nil
	}
	return CurlError(errno)
}

// curl_easy interface
type CURL struct {
	handle unsafe.Pointer
	// callback functions, bool ret means ok or not
	headerFunction, writeFunction *func([]byte, interface{}) bool
	readFunction                  *func([]byte, interface{}) int // return num of bytes writed to buf
	progressFunction              *func(float64, float64, float64, float64, interface{}) bool
	fnmatchFunction               *func(string, string, interface{}) int
	// callback datas
	headerData, writeData, readData, progressData, fnmatchData interface{}
	// list of C allocs
	mallocAllocs []*C.char
}

// concurrent safe context map
type contextMap struct {
	items map[uintptr]*CURL
	sync.RWMutex
}

func (c *contextMap) Set(k uintptr, v *CURL) {
	c.Lock()
	defer c.Unlock()

	c.items[k] = v
}

func (c *contextMap) Get(k uintptr) *CURL {
	c.RLock()
	defer c.RUnlock()

	return c.items[k]
}

func (c *contextMap) Delete(k uintptr) {
	c.Lock()
	defer c.Unlock()

	delete(c.items, k)
}

var context_map = &contextMap{
	items: make(map[uintptr]*CURL),
}

// curl_easy_init - Start a libcurl easy session
func EasyInit() *CURL {
	p := C.curl_easy_init()
	c := &CURL{handle: p, mallocAllocs: make([]*C.char, 0)} // other field defaults to nil
	context_map.Set(uintptr(p), c)
	return c
}

// curl_easy_duphandle - Clone a libcurl session handle
func (curl *CURL) Duphandle() *CURL {
	p := C.curl_easy_duphandle(curl.handle)
	c := &CURL{handle: p}
	context_map.Set(uintptr(p), c)
	return c
}

// curl_easy_cleanup - End a libcurl easy session
func (curl *CURL) Cleanup() {
	p := curl.handle
	C.curl_easy_cleanup(p)
	curl.MallocFreeAfter(0)
	context_map.Delete(uintptr(p))
}

// curl_easy_setopt - set options for a curl easy handle
// WARNING: a function pointer is &fun, but function addr is reflect.ValueOf(fun).Pointer()
func (curl *CURL) Setopt(opt int, param interface{}) error {
	p := curl.handle
	if param == nil {
		// NOTE: some option will crash program when got a nil param
		return newCurlError(C.curl_easy_setopt_pointer(p, C.CURLoption(opt), nil))
	}
	switch {
	// not really set
	case opt == OPT_READDATA: // OPT_INFILE
		curl.readData = param
		return nil
	case opt == OPT_PROGRESSDATA:
		curl.progressData = param
		return nil
	case opt == OPT_HEADERDATA: // also known as OPT_WRITEHEADER
		curl.headerData = param
		return nil
	case opt == OPT_WRITEDATA: // OPT_FILE
		curl.writeData = param
		return nil

	case opt == OPT_READFUNCTION:
		fun := param.(func([]byte, interface{}) int)
		curl.readFunction = &fun

		ptr := C.return_read_function()
		if err := newCurlError(C.curl_easy_setopt_pointer(p, C.CURLoption(opt), ptr)); err == nil {
			return newCurlError(C.curl_easy_setopt_pointer(p, OPT_READDATA, unsafe.Pointer(curl.handle)))
		} else {
			return err
		}

	case opt == OPT_PROGRESSFUNCTION:
		fun := param.(func(float64, float64, float64, float64, interface{}) bool)
		curl.progressFunction = &fun

		ptr := C.return_progress_function()
		if err := newCurlError(C.curl_easy_setopt_pointer(p, C.CURLoption(opt), ptr)); err == nil {
			return newCurlError(C.curl_easy_setopt_pointer(p, OPT_PROGRESSDATA, unsafe.Pointer(curl.handle)))
		} else {
			return err
		}

	case opt == OPT_HEADERFUNCTION:
		fun := param.(func([]byte, interface{}) bool)
		curl.headerFunction = &fun

		ptr := C.return_header_function()
		if err := newCurlError(C.curl_easy_setopt_pointer(p, C.CURLoption(opt), ptr)); err == nil {
			return newCurlError(C.curl_easy_setopt_pointer(p, OPT_HEADERDATA, unsafe.Pointer(curl.handle)))
		} else {
			return err
		}

	case opt == OPT_WRITEFUNCTION:
		fun := param.(func([]byte, interface{}) bool)
		curl.writeFunction = &fun

		ptr := C.return_write_function()
		if err := newCurlError(C.curl_easy_setopt_pointer(p, C.CURLoption(opt), ptr)); err == nil {
			return newCurlError(C.curl_easy_setopt_pointer(p, OPT_WRITEDATA, unsafe.Pointer(curl.handle)))
		} else {
			return err
		}

	// for OPT_HTTPPOST, use struct Form
	// MIME 结构在此处创建并挂到句柄上，之后 Form.Add* 只会追加字段。
	case opt == OPT_HTTPPOST || opt == OPT_MIMEPOST:
		form, ok := param.(*Form)
		if !ok || form == nil {
			return errors.New("curl: OPT_HTTPPOST expects *curl.Form obtained via curl.NewForm()")
		}
		return form.bind(curl)

	case opt >= C.CURLOPTTYPE_OFF_T:
		val := C.off_t(0)
		switch t := param.(type) {
		case int:
			val = C.off_t(t)
		case uint64:
			val = C.off_t(t)
		default:
			panic("OFF_T conversion not supported")
		}
		return newCurlError(C.curl_easy_setopt_off_t(p, C.CURLoption(opt), val))

	case opt >= C.CURLOPTTYPE_FUNCTIONPOINT:
		// function pointer
		panic("function pointer not implemented yet!")

	case opt >= C.CURLOPTTYPE_OBJECTPOINT:
		switch t := param.(type) {
		case string:
			ptr := C.CString(t)
			curl.mallocAddPtr(ptr)
			return newCurlError(C.curl_easy_setopt_string(p, C.CURLoption(opt), ptr))
		case CurlString:
			ptr := (*C.char)(t)
			return newCurlError(C.curl_easy_setopt_string(p, C.CURLoption(opt), ptr))
		case []string:
			if len(t) > 0 {
				ptr := C.CString(t[0])
				curl.mallocAddPtr(ptr)
				a_slist := C.curl_slist_append(nil, ptr)
				for _, s := range t[1:] {
					ptr := C.CString(s)
					curl.mallocAddPtr(ptr)
					a_slist = C.curl_slist_append(a_slist, ptr)
				}
				return newCurlError(C.curl_easy_setopt_slist(p, C.CURLoption(opt), a_slist))
			} else {
				return newCurlError(C.curl_easy_setopt_slist(p, C.CURLoption(opt), nil))
			}
		case []CurlString:
			if len(t) > 0 {
				ptr := (*C.char)(t[0])
				a_slist := C.curl_slist_append(nil, ptr)
				for _, s := range t[1:] {
					ptr := (*C.char)(s)
					a_slist = C.curl_slist_append(a_slist, ptr)
				}
				return newCurlError(C.curl_easy_setopt_slist(p, C.CURLoption(opt), a_slist))
			} else {
				return newCurlError(C.curl_easy_setopt_slist(p, C.CURLoption(opt), nil))
			}
		default:
			// It panics if v's Kind is not Chan, Func, Map, Ptr, Slice, or UnsafePointer.
			// val := reflect.ValueOf(param)
			//fmt.Printf("DEBUG(Setopt): param=%x\n", val.Pointer())
			//println("DEBUG can addr =", val.Pointer(), "opt=", opt)
			// pass a pointer to GoInterface
			return newCurlError(C.curl_easy_setopt_pointer(p, C.CURLoption(opt),
				unsafe.Pointer(&param)))
		}
	case opt >= C.CURLOPTTYPE_LONG:
		val := C.long(0)
		switch t := param.(type) {
		case int:
			val = C.long(t)
		case bool:
			if t {
				val = 1
			}
		case int64:
			val = C.long(t)
		case int32:
			val = C.long(t)
		default:
			panic("not supported converstion to c long")
		}
		return newCurlError(C.curl_easy_setopt_long(p, C.CURLoption(opt), val))
	}
	panic("opt param error!")
}

// curl_easy_send - sends raw data over an "easy" connection
func (curl *CURL) Send(buffer []byte) (int, error) {
	p := curl.handle
	buflen := len(buffer)
	n := C.size_t(0)
	ret := C.curl_easy_send(p, unsafe.Pointer(&buffer[0]), C.size_t(buflen), &n)
	return int(n), newCurlError(ret)
}

// curl_easy_recv - receives raw data on an "easy" connection
func (curl *CURL) Recv(buffer []byte) (int, error) {
	p := curl.handle
	buflen := len(buffer)
	buf := C.CString(string(buffer))
	n := C.size_t(0)
	ret := C.curl_easy_recv(p, unsafe.Pointer(buf), C.size_t(buflen), &n)
	return copy(buffer, C.GoStringN(buf, C.int(n))), newCurlError(ret)
}

// curl_easy_perform - Perform a file transfer
func (curl *CURL) Perform() error {
	p := curl.handle
	return newCurlError(C.curl_easy_perform(p))
}

// curl_easy_pause - pause and unpause a connection
func (curl *CURL) Pause(bitmask int) error {
	p := curl.handle
	return newCurlError(C.curl_easy_pause(p, C.int(bitmask)))
}

// curl_easy_reset - reset all options of a libcurl session handle
func (curl *CURL) Reset() {
	p := curl.handle
	C.curl_easy_reset(p)
}

// curl_easy_escape - URL encodes the given string
func (curl *CURL) Escape(url string) string {
	p := curl.handle
	oldUrl := C.CString(url)
	defer C.free(unsafe.Pointer(oldUrl))
	newUrl := C.curl_easy_escape(p, oldUrl, 0)
	defer C.curl_free(unsafe.Pointer(newUrl))
	return C.GoString(newUrl)
}

// curl_easy_unescape - URL decodes the given string
func (curl *CURL) Unescape(url string) string {
	p := curl.handle
	oldUrl := C.CString(url)
	outlength := C.int(0)
	defer C.free(unsafe.Pointer(oldUrl))
	// If outlength is non-NULL, the function will write the length of the
	// returned string in  the  integer  it  points  to.  This allows an
	// escaped string containing %00 to still get used properly after unescaping.
	newUrl := C.curl_easy_unescape(p, oldUrl, 0, &outlength)
	defer C.curl_free(unsafe.Pointer(newUrl))
	return C.GoStringN(newUrl, outlength)
}

// curl_easy_getinfo - extract information from a curl handle
func (curl *CURL) Getinfo(info CurlInfo) (ret interface{}, err error) {
	p := curl.handle
	cInfo := C.CURLINFO(info)
	switch cInfo & C.CURLINFO_TYPEMASK {
	case C.CURLINFO_STRING:
		a_string := C.CString("")
		defer C.free(unsafe.Pointer(a_string))
		err := newCurlError(C.curl_easy_getinfo_string(p, cInfo, &a_string))
		ret := C.GoString(a_string)
		debugf("Getinfo %s", ret)
		return ret, err
	case C.CURLINFO_LONG:
		a_long := C.long(-1)
		err := newCurlError(C.curl_easy_getinfo_long(p, cInfo, &a_long))
		ret := int(a_long)
		debugf("Getinfo %v", ret)
		return ret, err
	case C.CURLINFO_DOUBLE:
		a_double := C.double(0.0)
		err := newCurlError(C.curl_easy_getinfo_double(p, cInfo, &a_double))
		ret := float64(a_double)
		debugf("Getinfo %v", ret)
		return ret, err
	case C.CURLINFO_SLIST:
		a_ptr_slist := (*C.struct_curl_slist)(nil)
		err := newCurlError(C.curl_easy_getinfo_slist(p, cInfo, &a_ptr_slist))
		ret := []string{}
		for a_ptr_slist != nil {
			debugf("Getinfo %s %v", C.GoString(a_ptr_slist.data), a_ptr_slist.next)
			ret = append(ret, C.GoString(a_ptr_slist.data))
			a_ptr_slist = a_ptr_slist.next
		}
		return ret, err
	default:
		panic("error calling Getinfo\n")
	}
	panic("not implemented yet!")
	return nil, nil
}

func (curl *CURL) GetHandle() unsafe.Pointer {
	return curl.handle
}

func (curl *CURL) MallocGetPos() int {
	return len(curl.mallocAllocs)
}

func (curl *CURL) MallocFreeAfter(from int) {
	l := len(curl.mallocAllocs)
	for idx := from; idx < l; idx++ {
		C.free(unsafe.Pointer(curl.mallocAllocs[idx]))
		curl.mallocAllocs[idx] = nil
	}
	curl.mallocAllocs = curl.mallocAllocs[0:from]
}

func (curl *CURL) mallocAddPtr(ptr *C.char) {
	curl.mallocAllocs = append(curl.mallocAllocs, ptr)
}

// A multipart/formdata HTTP POST form.
//
// Form 使用 libcurl 的 MIME API（curl_mime_*）实现。旧的 curl_formadd /
// CURLFORM_* 接口自 libcurl 7.56.0 起已被废弃，在较新的发行版上会导致编译告警
// 甚至编译失败，因此这里完全迁移到 curl_mime。
//
// 使用方式：
//
//	easy := curl.EasyInit()
//	easy.Setopt(curl.OPT_URL, "https://example.com/upload")
//	form := curl.NewForm()
//	form.Add("field1", "value")
//	form.AddFile("file", "/path/to/file.bin")
//	easy.Setopt(curl.OPT_HTTPPOST, form)
//	defer form.Free()   // 在 easy.Cleanup() 之前调用
//	easy.Perform()
//
// 一个 Form 对应一份 MIME 结构，Add/AddWithType/AddFile 都追加到同一份结构中，
// 多次调用不会互相覆盖（旧实现每次 Add 都会新建并覆盖 CURLOPT_MIMEPOST，
// 导致只有最后一个字段生效）。
type Form struct {
	mime *C.curl_mime
}

// NewForm 创建一个新的 multipart/form-data 表单。
// 表单会在第一次 Add 时绑定到 CURL 句柄，因此无需在创建时传入句柄。
func NewForm() *Form {
	return &Form{}
}

// bind 确保已创建 MIME 结构并挂到指定句柄上（懒加载）。
// 后续成功绑定后，即使 Setopt 传入的 curl 参数不同也会沿用已绑定的句柄。
func (form *Form) bind(curl *CURL) error {
	if form.mime != nil {
		return nil
	}
	if curl == nil || curl.handle == nil {
		return errors.New("curl: Form requires a valid CURL handle")
	}
	m := C.curl_mime_create(curl.handle)
	if m == nil {
		return errors.New("curl: failed to create mime form")
	}
	form.mime = m
	return nil
}

// Free 释放 MIME 结构占用的 C 内存。
// 必须在 EasyCleanup 之前调用，且释放后不可再向该 Form 添加字段。
func (form *Form) Free() {
	C.curl_mime_free_form(form.mime)
	form.mime = nil
}

// Add 追加一个表单字段。content 支持 string 与 []byte，两者都会按原始字节发送。
func (form *Form) Add(name string, content interface{}) error {
	return form.AddWithType(name, content, "")
}

// AddWithType 追加一个带显式 Content-Type 的表单字段。
// contentType 为空字符串时由 libcurl 自行推断。
func (form *Form) AddWithType(name string, content interface{}, contentType string) error {
	if form.mime == nil {
		return errors.New("curl: Form is not bound to a CURL handle, call Setopt(OPT_HTTPPOST, form) first")
	}

	var buf unsafe.Pointer
	var length C.size_t

	switch t := content.(type) {
	case string:
		if len(t) > 0 {
			buf = C.CBytes([]byte(t))
			defer C.free(buf)
		}
		length = C.size_t(len(t))
	case []byte:
		if len(t) > 0 {
			buf = C.CBytes(t)
			defer C.free(buf)
		}
		length = C.size_t(len(t))
	default:
		return fmt.Errorf("curl: unsupported form content type %T (want string or []byte)", content)
	}

	namestr := C.CString(name)
	defer C.free(unsafe.Pointer(namestr))

	var typestr *C.char
	if contentType != "" {
		typestr = C.CString(contentType)
		defer C.free(unsafe.Pointer(typestr))
	}

	if ret := C.curl_mime_add_part_data(form.mime, namestr, (*C.char)(buf), length, typestr); ret != C.CURLE_OK {
		return fmt.Errorf("curl: failed to add form part %q: %s", name, C.GoString(C.curl_easy_strerror(ret)))
	}
	return nil
}

// AddFile 追加一个文件字段，使用文件的二进制内容作为字段值。
// Content-Type 会优先根据扩展名推断。
//
// 注意：这是一个会返回 error 的方法，符合 Go 的惯例；与上游历史版本中
// 无返回值、且内容为空的同名方法不兼容，调用方需要处理返回值。
func (form *Form) AddFile(name, filename string) error {
	return form.AddFileWithType(name, filename, guessType(filename))
}

// AddFileWithType 追加一个文件字段，并显式指定 Content-Type。
func (form *Form) AddFileWithType(name, filename, contentType string) error {
	if form.mime == nil {
		return errors.New("curl: Form is not bound to a CURL handle, call Setopt(OPT_HTTPPOST, form) first")
	}
	if filename == "" {
		return errors.New("curl: AddFile requires a non-empty filename")
	}

	namestr := C.CString(name)
	defer C.free(unsafe.Pointer(namestr))
	filestr := C.CString(filename)
	defer C.free(unsafe.Pointer(filestr))

	var typestr *C.char
	if contentType != "" {
		typestr = C.CString(contentType)
		defer C.free(unsafe.Pointer(typestr))
	}

	if ret := C.curl_mime_add_part_file(form.mime, namestr, filestr, nil, typestr); ret != C.CURLE_OK {
		return fmt.Errorf("curl: failed to add file part %q from %q: %s", name, filename, C.GoString(C.curl_easy_strerror(ret)))
	}
	return nil
}

// AddFileAs 追加一个文件字段，并以 remoteFilename 作为上传后的文件名。
func (form *Form) AddFileAs(name, filename, remoteFilename string) error {
	if form.mime == nil {
		return errors.New("curl: Form is not bound to a CURL handle, call Setopt(OPT_HTTPPOST, form) first")
	}
	if filename == "" {
		return errors.New("curl: AddFileAs requires a non-empty filename")
	}

	namestr := C.CString(name)
	defer C.free(unsafe.Pointer(namestr))
	filestr := C.CString(filename)
	defer C.free(unsafe.Pointer(filestr))
	remotestr := C.CString(remoteFilename)
	defer C.free(unsafe.Pointer(remotestr))
	typestr := C.CString(guessType(filename))
	defer C.free(unsafe.Pointer(typestr))

	if ret := C.curl_mime_add_part_file(form.mime, namestr, filestr, remotestr, typestr); ret != C.CURLE_OK {
		return fmt.Errorf("curl: failed to add file part %q from %q: %s", name, filename, C.GoString(C.curl_easy_strerror(ret)))
	}
	return nil
}

func guessType(filename string) string {
	ext := path.Ext(filename)
	file_type := mime.TypeByExtension(ext)
	if file_type == "" {
		return "application/octet-stream"
	}
	return file_type
}
