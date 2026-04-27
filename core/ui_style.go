package core

import (
	"fmt"
	"regexp"
	"strings"
)

// termuiStyleToTview converts [text](fg:COLOR,bg:COLOR,mod:bold) to tview [color]text[-] format.
func termuiStyleToTview(text string) string {
	re := regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		contentStart := strings.Index(match, "[") + 1
		contentEnd := strings.Index(match, "](")
		if contentEnd < 0 {
			return match
		}
		content := match[contentStart:contentEnd]

		styleStart := contentEnd + 2
		styleEnd := len(match) - 1
		if styleStart >= styleEnd {
			return match
		}
		style := match[styleStart:styleEnd]

		fg := ""
		bg := ""
		mod := ""

		parts := strings.Split(style, ",")
		for _, part := range parts {
			kv := strings.SplitN(part, ":", 2)
			key := strings.TrimSpace(kv[0])
			if len(kv) == 2 {
				value := strings.TrimSpace(kv[1])
				switch key {
				case "fg":
					fg = value
				case "bg":
					bg = value
				case "mod":
					mod = value
				}
			}
		}

		fg = mapColorName(fg)
		bg = mapColorName(bg)
		mod = mapModFlag(mod)

		var tag string
		if fg != "" && bg != "" && mod != "" {
			tag = fmt.Sprintf("[%s:%s:%s]", fg, bg, mod)
		} else if fg != "" && bg != "" {
			tag = fmt.Sprintf("[%s:%s]", fg, bg)
		} else if fg != "" && mod != "" {
			tag = fmt.Sprintf("[%s::%s]", fg, mod)
		} else if fg != "" {
			tag = fmt.Sprintf("[%s]", fg)
		} else if bg != "" && mod != "" {
			tag = fmt.Sprintf("[%s:%s]", bg, mod)
		} else if bg != "" {
			tag = fmt.Sprintf("[:%s]", bg)
		} else if mod != "" {
			tag = fmt.Sprintf("[::%s]", mod)
		} else {
			return content
		}

		return tag + content + "[-:-:-]"
	})
}

func mapColorName(name string) string {
	switch name {
	case "cyan":
		return "teal"
	case "magenta":
		return "purple"
	default:
		return name
	}
}

func mapModFlag(mod string) string {
	switch mod {
	case "bold":
		return "b"
	case "dim":
		return "d"
	case "italic":
		return "i"
	case "underline":
		return "u"
	case "strikethrough", "strike":
		return "s"
	case "blink":
		return "l"
	case "reverse":
		return "r"
	case "default":
		return ""
	default:
		return mod
	}
}
