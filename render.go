package teisai

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const (
	Bold int = iota
	Underline
	Italic
	Strike
)

const (
	SimpleLink   = `(?s)@D?\((.*?)\)`
	ComplexLink  = `(?s)@D?\[(.*?)\]\s*\((.*?)\)`
	NormalImg    = `(?s):\((.*?)\)`
	HiddenImg    = `(?s):\[(.*?)\]\s*\((.*?)\)`
	FootnoteRef  = `(?s)^\[\^([^\[]*?)\]: `
	FootnoteLink = `(?s)\[\^([^\[]*?)\]`
)

func GetParagraphs(str string) []string {
	strNormalized := regexp.
		MustCompile("\r\n").
		ReplaceAllString(str, "\n")

	return regexp.
		MustCompile(`\n\s*\n`).
		Split(strNormalized, -1)
}

func RenderBreaks(p string) string {
	scanner := bufio.NewScanner(strings.NewReader(p))
	t := ""
	suffix := " +"
	for scanner.Scan() {
		txt := scanner.Text()
		if strings.HasSuffix(txt, suffix) {
			t += strings.TrimSuffix(txt, suffix) + "<br>\n"
			continue
		}

		// typing ' +' is weird on a single line, allow just '+'
		if txt == strings.TrimSpace(suffix) {
			t += "<br>\n"
			continue
		}

		t += txt + "\n"
	}
	return t
}

func RenderHeader(p string) string {
	prefix := ""
	for i := 1; i <= 6; i++ {
		prefix += "#"
		h := strconv.Itoa(i)
		if !strings.HasPrefix(p, prefix+" ") {
			continue
		}

		p = strings.TrimPrefix(p, prefix+" ")
		p = "\\<h" + h + ">" + strings.TrimSpace(p) + "</h" + h + ">"
		break
	}

	return p
}

func RenderQuote(p string) string {
	prefix := "> "
	if !strings.HasPrefix(p, prefix) {
		return p
	}

	// remove all '> ' from the beginning of the line, then process the quotes
	// as separate paragraphs
	scanner := bufio.NewScanner(strings.NewReader(p))
	t := ""
	for scanner.Scan() {
		txt := scanner.Text()
		if strings.HasPrefix(txt, prefix) {
			t += strings.TrimPrefix(txt, prefix) + "\n"
			continue
		}

		// typing '> ' is weird on a single line, allow just '>'
		if txt == strings.TrimSpace(prefix) {
			t += "\n"
			continue
		}

		t += txt + "\n"
	}

	p = "\\<blockquote>\n" + RenderText(t) + "</blockquote>"
	return p
}

func RenderTable(p string) string {
	sep := "|"
	head := "!"
	if !strings.HasPrefix(p, sep) {
		return p
	}

	scanner := bufio.NewScanner(strings.NewReader(p))
	t := ""
	header := false
	if strings.HasPrefix(p, sep+head) {
		t = "<thead>"
		header = true
	} else {
		t = "<tbody>"
	}
	t += "\n"
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, sep)
		t += "<tr>\n"
		for i, f := range fields {
			if i == 0 {
				continue
			}
			if i == 1 && strings.HasPrefix(f, head) {
				f = strings.TrimSpace(strings.TrimPrefix(f, head))
			}
			if header {
				t += "<th>" + strings.TrimSpace(f) + "</th>\n"
			} else {
				t += "<td>" + strings.TrimSpace(f) + "</td>\n"
			}
		}
		t += "</tr>\n"
		if header {
			header = false
			t += "</thead>\n<tbody>\n"
		}
	}
	t += "</tbody>\n"

	p = "\\<table>\n" + t + "</table>"
	return p
}

func RenderList(p string, ordered bool) string {
	prefix := "* "
	tag := "ul"
	if ordered {
		prefix = "- "
		tag = "ol"
	}

	if !strings.HasPrefix(p, prefix) {
		return p
	}

	scanner := bufio.NewScanner(strings.NewReader(p))
	scanner.Scan()
	par := strings.TrimPrefix(scanner.Text(), prefix)
	t := "<li>"
	for scanner.Scan() {
		txt := scanner.Text()
		if !strings.HasPrefix(txt, prefix) {
			par += "\n" + txt
			continue
		}

		t += par + "</li>\n<li>"
		par = strings.TrimPrefix(txt, prefix)
	}
	t += par + "</li>"

	p = "\\<" + tag + ">\n" + t + "\n</" + tag + ">"
	return p
}

func RenderAccent(p string, a int) string {
	var reg, tag string
	switch a {
	case Bold:
		reg = `(?s)\*\*(.*?)\*\*`
		tag = "b"
	case Underline:
		reg = `(?s)\_\_(.*?)\_\_`
		tag = "u"
	case Italic:
		reg = `(?s)\~\~(.*?)\~\~`
		tag = "i"
	case Strike:
		reg = `(?s)\-\-(.*?)\-\-`
		tag = "s"
	}

	naccs := regexp.
		MustCompile(reg).
		FindAllStringSubmatch(p, -1)

	for _, n := range naccs {
		match, text := n[0], n[1]
		acc := `<` + tag + `>` + text + `</` + tag + `>`
		p = strings.Replace(p, match, acc, 1)
	}

	return p
}

func RenderLinks(p string) string {
	clinks := regexp.
		MustCompile(ComplexLink).
		FindAllStringSubmatch(p, -1)

	for _, l := range clinks {
		outside := false
		match, s1, s2 := l[0], l[1], l[2]
		if strings.HasPrefix(s2, "^") {
			s2 = strings.TrimPrefix(s2, "^")
			outside = true
		}
		link := `<a href="` + s2 + `"`
		if outside {
			link += ` target="_blank"`
		}
		if strings.HasPrefix(match, "@D") {
			link += ` download`
		}
		link += `>` + s1 + `</a>`
		p = strings.Replace(p, match, link, 1)
	}

	slinks := regexp.
		MustCompile(SimpleLink).
		FindAllStringSubmatch(p, -1)

	for _, l := range slinks {
		outside := false
		match, s1 := l[0], l[1]
		if strings.HasPrefix(s1, "^") {
			s1 = strings.TrimPrefix(s1, "^")
			outside = true
		}
		name := path.Base(s1)
		if strings.HasPrefix(s1, "http") {
			u, err := url.Parse(s1)
			if err != nil {
				fmt.Fprintln(os.Stderr, "RenderLinks: url parse error:", err)
			}
			name = u.Host
			name = strings.TrimPrefix(name, "www.")
		}
		link := `<a href="` + s1 + `"`
		if outside {
			link += ` target="_blank"`
		}
		if strings.HasPrefix(match, "@D") {
			link += ` download`
		}
		link += `>` + name + `</a>`

		p = strings.Replace(p, match, link, 1)
	}

	return p
}

func RenderImgs(p string) string {
	nimgs := regexp.
		MustCompile(NormalImg).
		FindAllStringSubmatch(p, -1)

	for _, n := range nimgs {
		var img string
		match, file := n[0], n[1]
		if strings.HasPrefix(file, "^") {
			file = strings.TrimPrefix(file, "^")
			img = `<a target="_blank" href="` + file + `"><img src="` + file + `" alt="` + file + `"></a>`
		} else {
			img = `<img src="` + file + `" alt="` + file + `">`
		}
		p = strings.Replace(p, match, img, 1)
	}

	himgs := regexp.
		MustCompile(HiddenImg).
		FindAllStringSubmatch(p, -1)

	for _, h := range himgs {
		match, desc, file := h[0], h[1], h[2]
		img := `\<details><summary>[` + desc + `]</summary><img src="` + file + `" alt="` + file + `"></details>`
		p = strings.Replace(p, match, img, 1)
	}

	return p
}

func RenderFootnotes(p string) string {
	nfs := regexp.
		MustCompile(FootnoteRef).
		FindStringSubmatch(p)

	if len(nfs) == 2 {
		match, ref := nfs[0], nfs[1]
		p = strings.TrimPrefix(p, match)
		p = `<p class="footnote" id="fn-` + ref + `">` + "\n" + `<sup><a href="#fr-` + ref + `">` + ref + "</a></sup>\n" + p + `</p>`
	}

	nfr := regexp.
		MustCompile(FootnoteLink).
		FindAllStringSubmatch(p, -1)

	for _, n := range nfr {
		match, ref := n[0], n[1]
		link := `<sup class="footref" id="fr-` + ref + `"><a href="#fn-` + ref + `">` + ref + "</a></sup>"
		p = strings.Replace(p, match, link, 1)
	}

	return p
}

func RenderParagraph(p string) string {
	if p == "===" {
		return "<hr>"
	}

	p = RenderBreaks(p)

	p = RenderHeader(p)
	p = RenderQuote(p)
	p = RenderTable(p)
	p = RenderList(p, true)
	p = RenderList(p, false)

	p = RenderAccent(p, Bold)
	p = RenderAccent(p, Underline)
	p = RenderAccent(p, Italic)
	p = RenderAccent(p, Strike)

	p = RenderLinks(p)
	p = RenderImgs(p)
	p = RenderFootnotes(p)

	p = strings.TrimPrefix(p, "\n")
	p = strings.TrimSuffix(p, "\n")

	// don't put <p> around escaped HTML tags
	if strings.HasPrefix(p, `\<`) {
		return p[1:]
	}

	return "<p>" + p + "</p>"
}

func GetMetadataFromReader(r io.Reader) (metadata map[string]string, ok bool) {
	metadata = make(map[string]string)
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	ok = false
	if scanner.Text() != "?" {
		return
	}
	ok = true

	for scanner.Scan() {
		txt := strings.TrimSpace(scanner.Text())
		if txt == "" {
			break
		}
		data := strings.SplitN(txt, "=", 2)
		if len(data) != 2 {
			fmt.Fprintln(os.Stderr, "GetMetadataFromReader: broken metadata:", txt)
			continue
		}
		metadata[data[0]] = data[1]
	}

	return
}

func GetMetadata(text string) (metadata map[string]string, ok bool) {
	return GetMetadataFromReader(strings.NewReader(text))
}

func ClearMetadata(text string) string {
	text = regexp.
		MustCompile("\r\n").
		ReplaceAllString(text, "\n")

	if len(text) < 2 {
		return text
	}

	if text[0] != '?' && text[1] != '\n' {
		return text
	}

	wasnl := true
	idx := -1
	for i, r := range text[2:] {
		if r != '\n' {
			wasnl = false
			continue
		}

		if !wasnl {
			wasnl = true
			continue
		}

		idx = i + 1
		break
	}
	if idx < 0 {
		return ""
	}

	return text[idx+2:]
}

func RenderText(text string) string {
	text = ClearMetadata(text)
	paragraphs := GetParagraphs(text)

	text = ""
	for _, p := range paragraphs {
		p := strings.TrimSpace(p)
		if p == "" {
			continue
		}
		text += RenderParagraph(p) + "\n"
	}
	return strings.TrimSpace(text)
}
