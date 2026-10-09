// Package secrets finds likely credentials in added diff lines before they are committed or pushed.
package secrets

import (
	"regexp"
	"strconv"
	"strings"
)

// Line is one added line of a diff.
type Line struct {
	File string
	Num  int // line number in the new file
	Text string
}

// Finding is a line that matched a rule.
type Finding struct {
	File    string
	Num     int
	Rule    string
	Excerpt string // the line with the secret value masked
}

type rule struct {
	name string
	re   *regexp.Regexp
	// value is the submatch index holding the secret (0 = whole match).
	value int
}

var rules = []rule{
	{"AWS access key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`), 0},
	{"GitHub token", regexp.MustCompile(`gh[po]_[A-Za-z0-9]{16,}`), 0},
	{"GitLab token", regexp.MustCompile(`glpat-[A-Za-z0-9_-]{16,}`), 0},
	{"SonarQube token", regexp.MustCompile(`squ_[0-9a-f]{20,}`), 0},
	{"OpenAI-style key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`), 0},
	{"Slack token", regexp.MustCompile(`xox[abp]-[A-Za-z0-9-]{10,}`), 0},
	{"private key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), 0},
	{"password/token assignment", regexp.MustCompile(`(?i)(?:_password|_authtoken|password|passwd|secret|token)["']?\s*[=:]\s*(\S+)`), 1},
}

// placeholder values that are not secrets: empty strings and references to variables.
func placeholder(v string) bool {
	v = strings.Trim(v, `"',;`)
	return v == "" || strings.HasPrefix(v, "$") || strings.HasPrefix(v, "%(") || strings.HasPrefix(v, "<")
}

// Scan returns one finding per line and rule match.
func Scan(lines []Line) []Finding {
	var out []Finding
	for _, l := range lines {
		for _, r := range rules {
			m := r.re.FindStringSubmatchIndex(l.Text)
			if m == nil {
				continue
			}
			start, end := m[2*r.value], m[2*r.value+1]
			if r.value > 0 && placeholder(l.Text[start:end]) {
				continue
			}
			out = append(out, Finding{File: l.File, Num: l.Num, Rule: r.name, Excerpt: mask(l.Text, start, end)})
		}
	}
	return out
}

// mask keeps the first four characters of the secret and hides the rest.
func mask(text string, start, end int) string {
	keep := min(start+4, end)
	return strings.TrimSpace(text[:keep] + strings.Repeat("*", min(end-keep, 12)) + text[end:])
}

var hunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// ParseDiff extracts added lines from unified diff output (any number of files/commits).
func ParseDiff(diff string) []Line {
	var out []Line
	file, num := "", 0
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "+++ "):
			file = strings.TrimPrefix(l, "+++ ")
			if file == "/dev/null" {
				file = ""
			}
			file = strings.TrimPrefix(file, "b/")
		case strings.HasPrefix(l, "@@"):
			if m := hunkRe.FindStringSubmatch(l); m != nil {
				num, _ = strconv.Atoi(m[1])
			}
		case strings.HasPrefix(l, "+") && file != "":
			out = append(out, Line{File: file, Num: num, Text: l[1:]})
			num++
		case strings.HasPrefix(l, " "):
			num++
		}
	}
	return out
}
