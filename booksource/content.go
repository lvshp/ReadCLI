package booksource

import (
	"bytes"
	stdhtml "html"
	"strings"

	"golang.org/x/net/html"
)

// NormalizeContent converts common HTML fragments to readable chapter text.
// Unknown angle-bracket words are preserved; this is not an HTML sanitizer.
// Entity decoding is bounded to three layers for escaped source responses.
func NormalizeContent(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for i := 0; i < 3; i++ {
		decoded := stdhtml.UnescapeString(text)
		if decoded == text {
			break
		}
		text = decoded
	}
	var out bytes.Buffer
	lineBreak := func(force bool) {
		// HTML indentation around block elements is not a paragraph of its own.
		for out.Len() > 0 {
			last := out.Bytes()[out.Len()-1]
			if last != ' ' && last != '\t' {
				break
			}
			out.Truncate(out.Len() - 1)
		}
		if out.Len() > 0 && (force || out.Bytes()[out.Len()-1] != '\n') {
			out.WriteByte('\n')
		}
	}
	tokenizer := html.NewTokenizer(strings.NewReader(text))
	hiddenTag := ""
	for {
		kind := tokenizer.Next()
		raw := tokenizer.Raw()
		if kind == html.ErrorToken {
			// The tokenizer can consume an unfinished tag before reaching EOF.
			if hiddenTag == "" {
				out.Write(raw)
			}
			break
		}
		if kind == html.TextToken {
			if hiddenTag == "" {
				out.Write(raw)
			}
			continue
		}
		// Token normalizes tag/attribute names in the tokenizer's Raw buffer.
		// Snapshot the spelling first so unknown literal words keep their case.
		original := string(raw)
		token := tokenizer.Token()
		if hiddenTag != "" {
			if kind == html.EndTagToken && token.Data == hiddenTag {
				hiddenTag = ""
			}
			continue
		}
		if kind == html.CommentToken {
			if !strings.HasPrefix(original, "<!--") {
				out.WriteString(original)
			}
			continue
		}
		if kind == html.DoctypeToken {
			continue
		}
		if !contentHTMLTag(token.Data) || !contentHTMLAttributes(token.Attr) {
			out.WriteString(original)
			continue
		}
		if token.Data == "script" || token.Data == "style" {
			if kind == html.StartTagToken {
				hiddenTag = token.Data
			}
			continue
		}
		// These elements otherwise expose their nested markup as a raw text
		// token, which would survive until a second normalization of the cache.
		if kind == html.StartTagToken {
			tokenizer.NextIsNotRawText()
		}
		if token.Data == "br" {
			if kind != html.EndTagToken {
				lineBreak(true)
			}
		} else if contentBlockTag(token.Data) {
			lineBreak(false)
		}
	}
	return strings.TrimSpace(out.String())
}

func contentHTMLTag(tag string) bool {
	switch tag {
	case "a", "abbr", "acronym", "address", "area", "article", "aside", "audio",
		"b", "base", "bdi", "bdo", "big", "blockquote", "body", "br", "button",
		"canvas", "caption", "center", "cite", "code", "col", "colgroup",
		"data", "datalist", "dd", "del", "details", "dfn", "dialog", "div", "dl", "dt",
		"em", "embed", "fieldset", "figcaption", "figure", "font", "footer", "form",
		"h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr", "html",
		"i", "iframe", "img", "input", "ins", "kbd", "label", "legend", "li", "link",
		"main", "map", "mark", "menu", "meta", "meter", "nav", "nobr", "noscript",
		"object", "ol", "optgroup", "option", "output", "p", "param", "picture", "pre", "progress",
		"q", "rp", "rt", "ruby", "s", "samp", "script", "section", "select", "small",
		"source", "span", "strike", "strong", "style", "sub", "summary", "sup",
		"table", "tbody", "td", "template", "textarea", "tfoot", "th", "thead", "time", "title", "tr", "track", "tt",
		"u", "ul", "var", "video", "wbr":
		return true
	}
	return false
}

func contentBlockTag(tag string) bool {
	switch tag {
	case "address", "article", "aside", "blockquote", "caption", "center", "dd", "details", "dialog", "div", "dl", "dt",
		"fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "hr",
		"li", "main", "menu", "nav", "ol", "p", "pre", "section", "summary", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul":
		return true
	}
	return false
}

// A comparison such as "a<b && c>d" is tokenized as a b element with malformed
// attributes. Keep that literal text rather than interpreting it as markup.
func contentHTMLAttributes(attrs []html.Attribute) bool {
	for _, attr := range attrs {
		for _, char := range attr.Key {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("-_:.", char)) {
				return false
			}
		}
	}
	return true
}
