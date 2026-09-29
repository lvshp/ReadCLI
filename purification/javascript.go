package purification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/dlclark/regexp2"
	"github.com/dop251/goja"
)

// Goja interrupts JavaScript loops, but its native RegExp calls do not check
// interrupts. Replace RegExp.prototype.exec so script-created regexes also use
// explicit timeouts. Goja's String.replace/match/search/split dispatch through
// this hook after the standard prototype is changed.
func installBoundedJSRegex(vm *goja.Runtime, ctx context.Context) error {
	prototype := vm.Get("RegExp").ToObject(vm).Get("prototype").ToObject(vm)
	type cachedRegex struct {
		regex *regexp2.Regexp
		names map[string]int
	}
	cache := make(map[string]cachedRegex)
	exec := func(call goja.FunctionCall) goja.Value {
		fail := func(err error) { panic(vm.NewGoError(err)) }
		if err := ctx.Err(); err != nil {
			fail(err)
		}
		object, validReceiver := call.This.(*goja.Object)
		if !validReceiver || object.ClassName() != "RegExp" {
			panic(vm.NewTypeError("RegExp method called on incompatible receiver"))
		}
		sourceValue, flagsValue := object.Get("source"), object.Get("flags")
		if sourceValue == nil || flagsValue == nil {
			panic(vm.NewTypeError("RegExp source or flags missing"))
		}
		pattern := sourceValue.String()
		flags := flagsValue.String()
		if len(pattern) > maxRuleBytes {
			fail(errors.New("JavaScript 正则过长"))
		}
		key := pattern + "\x00" + flags
		cached, ok := cache[key]
		if !ok {
			jsPattern := pattern
			if !strings.Contains(flags, "u") {
				var encoded strings.Builder
				for _, r := range pattern {
					if r > 0xffff {
						hi, lo := utf16.EncodeRune(r)
						fmt.Fprintf(&encoded, `\u%04x\u%04x`, hi, lo)
					} else {
						encoded.WriteRune(r)
					}
				}
				jsPattern = encoded.String()
			}
			converted, names, err := compatiblePattern(jsPattern, false)
			if err != nil {
				fail(errors.New("JavaScript 正则语法不支持"))
			}
			options := regexp2.RegexOptions(regexp2.ECMAScript)
			if strings.Contains(flags, "i") {
				options |= regexp2.IgnoreCase
			}
			if strings.Contains(flags, "m") {
				options |= regexp2.Multiline
			}
			if strings.Contains(flags, "s") {
				options |= regexp2.Singleline
			}
			if strings.Contains(flags, "u") {
				options |= regexp2.Unicode
			}
			regex, err := regexp2.Compile(converted, options)
			if err != nil {
				fail(errors.New("JavaScript 正则语法不支持"))
			}
			regex.MatchTimeout = matchTimeout
			if deadline, ok := ctx.Deadline(); ok {
				regex.MatchTimeout = min(regex.MatchTimeout, time.Until(deadline))
			}
			cached = cachedRegex{regex: regex, names: names}
			if len(cache) < 128 {
				cache[key] = cached
			}
		}
		input := call.Argument(0).ToString().(goja.String)
		if input.Length() > maxTextBytes {
			fail(errors.New("JavaScript 正则输入过长"))
		}
		units := make([]uint16, input.Length())
		for i := range units {
			units[i] = input.CharAt(i)
		}
		runes := make([]rune, len(units))
		positions := make([]int, len(units)+1)
		if strings.Contains(flags, "u") {
			runes = nil
			positions = []int{0}
			for i := 0; i < len(units); i++ {
				r := rune(units[i])
				if r >= 0xd800 && r <= 0xdbff && i+1 < len(units) && units[i+1] >= 0xdc00 && units[i+1] <= 0xdfff {
					r = utf16.DecodeRune(r, rune(units[i+1]))
					i++
				}
				runes = append(runes, r)
				positions = append(positions, i+1)
			}
		} else {
			for i, c := range units {
				runes[i] = rune(c)
				positions[i] = i
			}
			positions[len(units)] = len(units)
		}
		global, sticky := strings.Contains(flags, "g"), strings.Contains(flags, "y")
		start := int64(0)
		if global || sticky {
			start = max(object.Get("lastIndex").ToInteger(), 0)
		}
		if start > int64(len(units)) {
			_ = object.Set("lastIndex", 0)
			return goja.Null()
		}
		startRune := 0
		for startRune < len(runes) && int64(positions[startRune]) < start {
			startRune++
		}
		match, err := cached.regex.FindRunesMatchStartingAt(runes, startRune)
		if err != nil {
			fail(context.DeadlineExceeded)
		}
		if err := ctx.Err(); err != nil {
			fail(err)
		}
		if match == nil || (sticky && match.Index != startRune) {
			if global || sticky {
				_ = object.Set("lastIndex", 0)
			}
			return goja.Null()
		}
		if global || sticky {
			_ = object.Set("lastIndex", positions[match.Index+match.Length])
		}
		values := make([]any, len(cached.regex.GetGroupNumbers()))
		for i := range values {
			group := match.GroupByNumber(i)
			if group == nil || len(group.Captures) == 0 {
				values[i] = goja.Undefined()
			} else {
				values[i] = input.Substring(positions[group.Index], positions[group.Index+group.Length])
			}
		}
		array := vm.NewArray(values...)
		_ = array.Set("index", positions[match.Index])
		_ = array.Set("input", input)
		if len(cached.names) > 0 {
			groups := vm.NewObject()
			for name, number := range cached.names {
				_ = groups.Set(name, values[number])
			}
			_ = array.Set("groups", groups)
		} else {
			_ = array.Set("groups", goja.Undefined())
		}
		return array
	}
	if err := prototype.DefineDataProperty("exec", vm.ToValue(exec), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
		return err
	}
	// This Goja version's native test bypasses prototype.exec.
	test := func(call goja.FunctionCall) goja.Value { return vm.ToValue(!goja.IsNull(exec(call))) }
	return prototype.DefineDataProperty("test", vm.ToValue(test), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
}
