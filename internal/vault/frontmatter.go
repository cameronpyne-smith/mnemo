package vault

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var fmDelimiter = []byte("---")

func ParseNote(content []byte) (Frontmatter, string, error) {
	var fm Frontmatter
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))

	rest, ok := bytes.CutPrefix(normalized, append(fmDelimiter, '\n'))
	if !ok {
		return fm, string(normalized), nil
	}

	end := bytes.Index(rest, append([]byte("\n"), append(fmDelimiter, '\n')...))
	var yamlPart, body []byte
	switch {
	case end >= 0:
		yamlPart = rest[:end]
		body = rest[end+len(fmDelimiter)+2:]
	case bytes.HasSuffix(rest, append([]byte("\n"), fmDelimiter...)):
		yamlPart = rest[:len(rest)-len(fmDelimiter)-1]
	default:
		return fm, string(normalized), nil
	}

	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return fm, "", fmt.Errorf("parsing frontmatter: %w", err)
	}
	return fm, strings.TrimPrefix(string(body), "\n"), nil
}

func EncodeNote(fm Frontmatter, body string) ([]byte, error) {
	yamlPart, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("encoding frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(fmDelimiter)
	buf.WriteByte('\n')
	buf.Write(yamlPart)
	buf.Write(fmDelimiter)
	buf.WriteByte('\n')
	if body != "" {
		buf.WriteByte('\n')
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes(), nil
}
