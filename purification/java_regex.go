package purification

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dlclark/regexp2"
)

const horizontalSpace = `\t \u00a0\u1680\u180e\u2000-\u200a\u202f\u205f\u3000`
const verticalSpace = `\n\u000b\f\r\u0085\u2028\u2029`

// javaPattern adapts syntax without mutating the user's stored expression.
func javaPattern(pattern string) (string, map[string]int, error) {
	return compatiblePattern(pattern, true)
}

func compatiblePattern(pattern string, java bool) (string, map[string]int, error) {
	var output strings.Builder
	names := make(map[string]int)
	group := 0
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '\\' && i+1 < len(pattern) {
			i++
			if !java {
				output.WriteByte('\\')
				output.WriteByte(pattern[i])
				continue
			}
			switch pattern[i] {
			case 'h':
				output.WriteString("[" + horizontalSpace + "]")
			case 'H':
				output.WriteString("[^" + horizontalSpace + "]")
			case 'v':
				output.WriteString("[" + verticalSpace + "]")
			case 'V':
				output.WriteString("[^" + verticalSpace + "]")
			case 'R':
				output.WriteString(`(?:\r\n|[` + verticalSpace + `])`)
			case 'Q':
				end := strings.Index(pattern[i+1:], `\E`)
				if end < 0 {
					output.WriteString(regexp2.Escape(pattern[i+1:]))
					i = len(pattern)
				} else {
					end += i + 1
					output.WriteString(regexp2.Escape(pattern[i+1 : end]))
					i = end + 1
				}
			default:
				output.WriteByte('\\')
				output.WriteByte(pattern[i])
			}
			continue
		}
		if ch == '[' {
			end := classEnd(pattern, i)
			if !java {
				end = -1
				for j := i + 1; j < len(pattern); j++ {
					if pattern[j] == '\\' {
						j++
						continue
					}
					if pattern[j] == ']' {
						end = j
						break
					}
				}
			}
			if end < 0 {
				return "", nil, errors.New("字符类未闭合")
			}
			if !java {
				output.WriteString(pattern[i : end+1])
				i = end
				continue
			}
			converted, err := javaClass(pattern[i+1 : end])
			if err != nil {
				return "", nil, err
			}
			output.WriteString(converted)
			i = end
			continue
		}
		if ch == '(' {
			if i+1 >= len(pattern) || pattern[i+1] != '?' {
				group++
				fmt.Fprintf(&output, "(?<%d>", group)
				continue
			}
			if strings.HasPrefix(pattern[i:], "(?<") && i+3 < len(pattern) && pattern[i+3] != '=' && pattern[i+3] != '!' {
				end := strings.IndexByte(pattern[i+3:], '>')
				if end < 0 {
					return "", nil, errors.New("命名捕获组未闭合")
				}
				end += i + 3
				name := pattern[i+3 : end]
				if name == "" || names[name] != 0 {
					return "", nil, errors.New("命名捕获组无效或重复")
				}
				group++
				names[name] = group
				fmt.Fprintf(&output, "(?<%d>", group)
				i = end
				continue
			}
		}
		output.WriteByte(ch)
	}
	converted := output.String()
	for name, number := range names {
		converted = strings.ReplaceAll(converted, `\k<`+name+`>`, fmt.Sprintf(`\k<%d>`, number))
	}
	return converted, names, nil
}

func classEnd(pattern string, start int) int {
	depth := 1
	for i := start + 1; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] == '[' {
			depth++
		}
		if pattern[i] == ']' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func javaClass(content string) (string, error) {
	// Java's [allowed&&[^excluded]] is an intersection, whereas regexp2
	// would silently interpret && as literal ampersands.
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\\' {
			i++
			continue
		}
		if content[i] == '[' {
			depth++
		}
		if content[i] == ']' {
			depth--
		}
		if depth == 0 && strings.HasPrefix(content[i:], "&&") {
			parts = append(parts, content[start:i])
			i++
			start = i + 1
		}
	}
	if len(parts) > 0 {
		parts = append(parts, content[start:])
		var result strings.Builder
		result.WriteString("(?:")
		for _, part := range parts {
			if strings.HasPrefix(part, "[") && classEnd(part, 0) == len(part)-1 {
				part = part[1 : len(part)-1]
			}
			class, err := javaClass(part)
			if err != nil {
				return "", err
			}
			result.WriteString("(?=" + class + ")")
		}
		result.WriteString(`[\s\S])`)
		return result.String(), nil
	}
	var output strings.Builder
	output.WriteByte('[')
	for i := 0; i < len(content); i++ {
		if content[i] == '\\' && i+1 < len(content) {
			i++
			switch content[i] {
			case 'h':
				output.WriteString(horizontalSpace)
			case 'v':
				output.WriteString(verticalSpace)
			case 'H', 'V', 'R':
				return "", errors.New("不支持字符类内的该空白转义，请改为否定字符类")
			default:
				output.WriteByte('\\')
				output.WriteByte(content[i])
			}
		} else {
			output.WriteByte(content[i])
		}
	}
	output.WriteByte(']')
	return output.String(), nil
}

type replacementPart struct {
	literal string
	group   int
}

func parseReplacement(value string, groupCount int, names map[string]int) ([]replacementPart, error) {
	var parts []replacementPart
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			parts = append(parts, replacementPart{literal: literal.String(), group: -1})
			literal.Reset()
		}
	}
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			i++
			if i >= len(value) {
				return nil, errors.New("替换内容以不完整转义结尾")
			}
			literal.WriteByte(value[i])
		case '$':
			flush()
			i++
			if i >= len(value) {
				return nil, errors.New("替换捕获组引用不完整")
			}
			group := -1
			if value[i] == '{' {
				end := strings.IndexByte(value[i+1:], '}')
				if end < 0 {
					return nil, errors.New("命名替换组未闭合")
				}
				end += i + 1
				var ok bool
				group, ok = names[value[i+1:end]]
				if !ok {
					return nil, errors.New("替换内容引用不存在的命名组")
				}
				i = end
			} else if value[i] >= '0' && value[i] <= '9' {
				group = int(value[i] - '0')
				if group > groupCount {
					return nil, errors.New("替换内容引用不存在的捕获组")
				}
				for i+1 < len(value) && value[i+1] >= '0' && value[i+1] <= '9' {
					next := group*10 + int(value[i+1]-'0')
					if next > groupCount {
						break
					}
					group = next
					i++
				}
			} else {
				return nil, errors.New("美元符号必须引用捕获组或使用反斜杠转义")
			}
			parts = append(parts, replacementPart{group: group})
		default:
			literal.WriteByte(value[i])
		}
	}
	flush()
	return parts, nil
}
