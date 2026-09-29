package booksource

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dop251/goja"
)

var ErrJSTimeout = errors.New("JavaScript执行超时")

// EvalJS exposes only portable helpers and explicitly granted JS bindings.
// Android/Rhino-specific APIs fail at the point of use with a named error.
func (e *Evaluator) EvalJS(code string, result any) (output any, evalErr error) {
	e.mu.RLock()
	timeout, library := e.jsTimeout, e.jsLib
	ctx := e.jsContext
	variables := make(map[string]any, len(e.variables))
	for key, value := range e.variables {
		variables[key] = value
	}
	bindings := make(map[string]any, len(e.jsBindings))
	for key, value := range e.jsBindings {
		bindings[key] = value
	}
	e.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("JavaScript请求已取消: %w", err)
	}
	defer func() {
		// A native binding may return a wrapped JS exception or catch its own
		// request error. Cancellation still ends the enclosing evaluation.
		if err := ctx.Err(); err != nil {
			output = nil
			evalErr = fmt.Errorf("JavaScript请求已取消: %w", err)
		}
	}()
	vm := goja.New()
	vm.SetMaxCallStackSize(256)
	for key, value := range variables {
		if err := vm.Set(key, jsData(value)); err != nil {
			return nil, fmt.Errorf("JS变量 %s: %w", key, err)
		}
	}
	_ = vm.Set("baseUrl", e.baseURL)
	_ = vm.Set("result", jsData(result))
	_ = vm.Set("src", jsData(result))
	host := vm.NewObject()
	_ = host.Set("get", func(key string) any { e.mu.RLock(); defer e.mu.RUnlock(); return jsData(e.variables[key]) })
	_ = host.Set("put", func(key string, value goja.Value) any {
		data := value.Export()
		e.mu.Lock()
		e.variables[key] = data
		e.mu.Unlock()
		return data
	})
	_ = host.Set("base64Decode", func(value string) (string, error) {
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(value)
		}
		return string(data), err
	})
	_ = host.Set("base64Encode", func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) })
	_ = host.Set("hexDecodeToString", func(value string) (string, error) { data, err := hex.DecodeString(value); return string(data), err })
	_ = host.Set("hexEncodeToString", func(value string) string { return hex.EncodeToString([]byte(value)) })
	_ = host.Set("md5Encode", func(value string) string { hash := md5.Sum([]byte(value)); return hex.EncodeToString(hash[:]) })
	_ = host.Set("encodeURI", func(value string) string { return url.QueryEscape(value) })
	_ = host.Set("decodeURI", func(value string) (string, error) { return url.QueryUnescape(value) })
	_ = host.Set("htmlFormat", func(value string) (string, error) {
		node, err := htmlNode(value)
		if err != nil {
			return "", err
		}
		return readableText(node, false), nil
	})
	_ = host.Set("log", func(goja.Value) {})
	for key, value := range bindings {
		if key == "java" {
			methods, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("JS宿主java必须是map[string]any")
			}
			for name, method := range methods {
				if err := host.Set(name, method); err != nil {
					return nil, fmt.Errorf("JS宿主java.%s: %w", name, err)
				}
			}
			continue
		}
		if err := vm.Set(key, value); err != nil {
			return nil, fmt.Errorf("JS宿主%s: %w", key, err)
		}
	}
	_ = vm.Set("__java", host)
	_, err := vm.RunString(`var java = new Proxy(__java, {get: function(target, key) { if (key in target) return target[key]; throw new Error("不支持的Java宿主方法: java." + String(key)); }}); delete this.__java;
var Packages = new Proxy({}, {get: function(_, key) {throw new Error("不支持的Java宿主功能: Packages." + String(key));}});
var importClass = function(){throw new Error("不支持的Java宿主功能: importClass");};
var importPackage = function(){throw new Error("不支持的Java宿主功能: importPackage");};
// An empty importer is a scope container used by compatibility detection.
// Importing an actual Java class remains unsupported.
var JavaImporter = function(){if(arguments.length) throw new Error("不支持的Java宿主功能: JavaImporter classes"); this.importClass = importClass; this.importPackage = importPackage;};`)
	if err != nil {
		return nil, err
	}
	// The timer covers source libraries as well as the actual expression.
	timer := time.AfterFunc(timeout, func() { vm.Interrupt(ErrJSTimeout) })
	defer timer.Stop()
	stopCancel := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()) })
	defer stopCancel()
	if strings.TrimSpace(library) != "" {
		if _, err = vm.RunString(library); err != nil {
			return nil, jsError("公共库", err)
		}
	}
	program, err := goja.Compile("booksource.js", code, false)
	if err != nil && strings.Contains(err.Error(), "Illegal return statement") {
		program, err = goja.Compile("booksource.js", "(function(){\n"+code+"\n}).call(this)", false)
	}
	if err != nil {
		return nil, jsError("语法", err)
	}
	value, err := vm.RunProgram(program)
	if err != nil {
		return nil, jsError("执行", err)
	}
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return nil, nil
	}
	return exportJSValue(value)
}

// Export invokes enumerable getters after RunProgram has returned. Goja throws
// these exceptions as panics outside its execution boundary, including an
// interrupt from a looping getter, so preserve the same error contract here.
func exportJSValue(value goja.Value) (output any, err error) {
	defer func() {
		switch failure := recover().(type) {
		case nil:
		case *goja.Exception:
			output = nil
			err = jsError("结果导出", failure)
		case *goja.InterruptedError:
			output = nil
			err = jsError("结果导出", failure)
		case *goja.StackOverflowError:
			output = nil
			err = jsError("结果导出", failure)
		default:
			panic(failure)
		}
	}()
	return value.Export(), nil
}

func jsError(stage string, err error) error {
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		if cause, ok := interrupted.Value().(error); ok {
			return fmt.Errorf("JavaScript%s中断: %w", stage, cause)
		}
		return fmt.Errorf("JavaScript%s中断: %w", stage, err)
	}
	return fmt.Errorf("JavaScript%s失败: %w", stage, err)
}

// JSON round-tripping prevents source scripts from discovering exported Go
// methods through a value passed by the caller.
func jsData(value any) any {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string, bool, float64, float32, int, int64, int32, uint, uint64:
		return v
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = jsData(item)
		}
		return out
	}
	data, err := json.Marshal(value)
	if err != nil {
		return stringify(value)
	}
	var out any
	if err = json.Unmarshal(data, &out); err != nil {
		return stringify(value)
	}
	return out
}
